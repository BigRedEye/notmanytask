package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/bigredeye/notmanytask/api"
	"github.com/bigredeye/notmanytask/internal/scorer"
)

var log *zap.Logger

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func unwrap[T any](value T, err error) T {
	check(err)
	return value
}

func LoadStandings(endpoint string) (*api.StandingsResponse, error) {
	req := unwrap(http.NewRequest("GET", fmt.Sprintf("%s/api/standings", endpoint), nil))
	req.Header.Add("Token", os.Getenv("NOTMANYTASK_TOKEN"))
	res := unwrap(http.DefaultClient.Do(req))
	body := unwrap(io.ReadAll(res.Body))

	var parsed api.StandingsResponse
	check(json.Unmarshal(body, &parsed))

	if !parsed.Ok {
		return nil, fmt.Errorf("failed to fetch standings: %s", parsed.Error)
	}

	return &parsed, nil
}

func run(fmtline string, arg ...interface{}) ([]byte, error) {
	line := fmt.Sprintf(fmtline, arg...)
	log.Debug("Running command", zap.String("cmd", line))

	cmd := exec.Command("bash", "-c", line)

	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()

	if err != nil {
		log.Error("Command failed",
			zap.String("cmd", line),
			zap.Error(err),
			zap.ByteString("stderr", stderr.Bytes()),
			zap.ByteString("stdout", stdout),
		)
		return nil, err
	}

	log.Info("Command finished", zap.String("cmd", line))
	return stdout, nil
}

func fetch(url, branch, output string) error {
	log.Info("Downloading repo", zap.String("url", url), zap.String("branch", branch), zap.String("dest", output))

	_, err := run("git clone %s --branch=%s %s", url, branch, output)
	if err != nil {
		log.Error("Failed to download repo",
			zap.Error(err),
			zap.String("url", url),
			zap.String("branch", branch),
		)
	} else {
		log.Info("Downloaded repo", zap.String("url", url), zap.String("branch", branch))
	}
	return err
}

func FetchSubmit(group, project, task, dest string) error {
	return fetch(
		fmt.Sprintf("git@gitlab.com:%s/%s.git", group, project),
		fmt.Sprintf("submits/%s", task),
		fmt.Sprintf("%s/%s/%s", dest, project, task),
	)
}

func FetchSubmits(group, endpoint, taskName string) ([]string, error) {
	standings, err := LoadStandings(endpoint)
	if err != nil {
		return nil, err
	}

	s := semaphore.NewWeighted(100)
	g := errgroup.Group{}
	count := 0

	branches := []string{}
	for _, user := range standings.Standings.Users {
		for _, grp := range user.Groups {
			for _, task := range grp.Tasks {
				if task.Task != taskName {
					continue
				}

				if task.Status != scorer.TaskStatusSuccess || task.Score <= 0 {
					log.Info("Skipping user",
						zap.String("login", user.User.GitlabLogin),
						zap.String("task_status", task.Status),
						zap.Int("task_score", task.Score),
					)
					continue
				}

				prj := user.User.GitlabProject
				name := task.Task
				branches = append(branches, fmt.Sprintf("%s/%s/%s", "solutions", prj, name))
				g.Go(func() error {
					s.Acquire(context.TODO(), 1)
					defer s.Release(1)
					return FetchSubmit(group, prj, name, "solutions")
				})
				count += 1
			}
		}
	}

	log.Info("Waiting for fetchers to complete", zap.Int("count", count))

	err = g.Wait()
	if err != nil {
		return nil, err
	}
	return branches, nil
}

func BuildSubmits(group, endpoint, task, target string) error {
	solutions, err := FetchSubmits(group, endpoint, task)
	if err != nil {
		return err
	}

	err = fetch("git@gitlab.com:danlark/cpp-advanced-hse.git", "main", "repo")
	if err != nil {
		return err
	}

	for _, solution := range solutions {
		unwrap(run("cd repo && git reset --hard"))
		unwrap(run("cd repo && git clean -xdf"))
		unwrap(run("cp -r %s %s", solution, "repo"))
		unwrap(run("cd repo && rm -rf build"))
		unwrap(run("cd repo && cmake -B build -DCMAKE_BUILD_TYPE=ASAN"))
		unwrap(run("cd repo && cmake --build build --parallel --target %s", target))
		unwrap(run("mkdir -p bins/%s", solution))
		unwrap(run("cp repo/build/%s bins/%s/%s", target, solution, target))

		log.Info("Built solution", zap.String("solution", solution))
	}

	return nil
}

var (
	args struct {
		GitlabGroup string
		Endpoint    string
		TaskName    string
		MainRepo    string
		TaskTarget  string
	}

	RootCmd = &cobra.Command{
		Use:   "fuzz",
		Short: "Run fuzz tests",
	}

	FetchCmd = &cobra.Command{
		Use:   "fetch",
		Short: "Fetch submits",
		RunE: func(cmd *cobra.Command, _args []string) error {
			_, err := FetchSubmits(args.GitlabGroup, args.Endpoint, args.TaskName)
			return err
		},
	}

	BuildCmd = &cobra.Command{
		Use:   "build",
		Short: "Build submits",
		RunE: func(cmd *cobra.Command, _args []string) error {
			return BuildSubmits(args.GitlabGroup, args.Endpoint, args.TaskName, args.TaskTarget)
		},
	}
)

func initLogging() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.ConsoleSeparator = " "
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.StampMilli)
	log = unwrap(config.Build())
}

func initCommands() {
	RootCmd.PersistentFlags().StringVar(&args.Endpoint, "endpoint", "https://cpp-hse.net", "Scoring system endpoint")
	RootCmd.PersistentFlags().StringVar(&args.GitlabGroup, "gitlab-group", "cpp-advanced-hse-2022", "Gitlab group name")
	RootCmd.PersistentFlags().StringVar(&args.TaskName, "task", "", "Task name")
	RootCmd.PersistentFlags().StringVar(&args.TaskTarget, "target", "", "Build target")

	RootCmd.AddCommand(FetchCmd)
	RootCmd.AddCommand(BuildCmd)
}

func init() {
	initLogging()
	initCommands()
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Command failed: %s", err.Error())
		os.Exit(1)
	}
}
