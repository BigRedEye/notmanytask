package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"reflect"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

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

type runOption func(*exec.Cmd)

func WithDir(dir string) runOption {
	return func(c *exec.Cmd) {
		c.Dir = dir
	}
}

func WithStderr(file string) runOption {
	_ = os.MkdirAll(path.Dir(file), 0755)
	w, err := os.Create(file)
	if err != nil {
		panic(err)
	}
	return func(c *exec.Cmd) {
		c.Stderr = w
	}
}

func WithStdout(file string) runOption {
	_ = os.MkdirAll(path.Dir(file), 0755)
	w, err := os.Create(file)
	if err != nil {
		panic(err)
	}
	return func(c *exec.Cmd) {
		c.Stdout = w
	}
}

func Run(fmtline string, opt ...interface{}) (string, error) {
	args := []interface{}{}
	opts := []runOption{}
	for _, arg := range opt {
		var f runOption
		if reflect.TypeOf(arg).ConvertibleTo(reflect.TypeOf(f)) {
			opts = append(opts, arg.(runOption))
		} else {
			args = append(args, arg)
		}
	}
	line := fmt.Sprintf(fmtline, args...)
	log.Debug("Running command", zap.String("cmd", line))

	cmd := exec.Command("bash", "-c", line)

	for _, opt := range opts {
		opt(cmd)
	}

	stderr := bytes.Buffer{}
	if cmd.Stderr == nil {
		cmd.Stderr = &stderr
	} else {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, &stderr)
	}

	stdout := bytes.Buffer{}
	if cmd.Stdout == nil {
		cmd.Stdout = &stderr
	} else {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, &stderr)
	}

	err := cmd.Run()

	if err != nil {
		log.Error("Command failed",
			zap.String("cmd", line),
			zap.Error(err),
			zap.ByteString("stderr", stderr.Bytes()),
			zap.ByteString("stdout", stdout.Bytes()),
		)
		return "", err
	}

	log.Info("Command finished", zap.String("cmd", line))
	return stdout.String(), nil
}

func MustRun(fmtline string, opt ...interface{}) string {
	return unwrap(Run(fmtline, opt...))
}

func fetch(url, branch, output string) error {
	log.Info("Downloading repo", zap.String("url", url), zap.String("branch", branch), zap.String("dest", output))

	_ = os.RemoveAll(output)
	_, err := Run("git clone %s --branch=%s %s", url, branch, output)
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

func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func FetchSubmit(group, project, task, dest string, nocache bool) error {
	output := fmt.Sprintf("%s/%s/%s", dest, project, task)

	exists, err := Exists(output)
	if !nocache && exists && err == nil {
		return nil
	}

	return fetch(
		fmt.Sprintf("git@gitlab.com:%s/%s.git", group, project),
		fmt.Sprintf("submits/%s", task),
		output,
	)
}

func FetchSubmits(args *Args) ([]string, error) {
	standings, err := LoadStandings(args.Endpoint)
	if err != nil {
		return nil, err
	}

	g := errgroup.Group{}
	g.SetLimit(100)
	count := 0

	branches := []string{}
	for _, user := range standings.Standings.Users {
		for _, grp := range user.Groups {
			for _, task := range grp.Tasks {
				if task.Task != args.TaskName {
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
					return FetchSubmit(args.GitlabGroup, prj, name, "solutions", args.NoCache)
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

func BuildSubmits(args *Args) (map[string]string, error) {
	solutions, err := FetchSubmits(args)
	if err != nil {
		return nil, err
	}

	err = fetch("git@gitlab.com:danlark/cpp-advanced-hse.git", "main", "repo")
	if err != nil {
		return nil, err
	}

	g := errgroup.Group{}
	g.SetLimit(10)

	bins := make(map[string]string)

	for _, s := range solutions {
		solution := s
		bin := fmt.Sprintf("bins/%s/%s", solution, args.TaskTarget)
		bins[solution] = bin
		g.Go(func() error {
			build := fmt.Sprintf("build/%s", solution)
			MustRun("mkdir -p %s", build)
			MustRun("rsync -a repo/ %s", build)
			MustRun("rsync -a %s/ %s/tasks", solution, build)
			MustRun("rm -rf build", WithDir(build))
			MustRun("cmake -B build -DCMAKE_BUILD_TYPE=ASAN", WithDir(build))
			MustRun("cmake --build build --target %s --parallel 4", args.TaskTarget, WithDir(build))
			MustRun("mkdir -p bins/%s", solution)
			MustRun("cp %s/build/%s %s", build, args.TaskTarget, bin)

			log.Info("Built solution", zap.String("solution", solution))
			return nil
		})
	}

	err = g.Wait()
	if err != nil {
		return nil, err
	}

	return bins, nil
}

func RunFuzzing(args *Args) error {
	bins, err := BuildSubmits(args)
	if err != nil {
		log.Error("Failed to build submits", zap.Error(err))
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(len(bins))

	stats := make(map[string]*bool)

	for k, v := range bins {
		finished := false
		stats[k] = &finished

		go func(solution string, bin string) {
			start := time.Now()
			l := log.With(zap.String("solution", solution))
			l.Info("Start fuzzing solution")
			_, err := Run(
				"%s %s -fork=%d -timeout=600 -max_total_time=%d -reload", bin, args.Corpus, args.Jobs, int(args.Timeout.Seconds()),
				WithStderr(fmt.Sprintf("logs/%s.err", solution)),
				WithStdout(fmt.Sprintf("logs/%s.out", solution)),
			)
			if err != nil {
				l.Error("Solution failed", zap.Error(err), zap.Duration("duration", time.Since(start)))
			} else {
				finished = true
				l.Info("Solution finished")
			}
			wg.Done()
		}(k, v)
	}

	log.Info("Started fuzzing workers", zap.Int("count", len(bins)))

	wg.Wait()

	for solution, ok := range stats {
		if *ok {
			fmt.Printf("Solution %s completed successfully!\n", solution)
		} else {
			fmt.Printf("Solution %s failed:(\n", solution)
		}
	}

	return nil
}

type Args struct {
	NoCache     bool
	GitlabGroup string
	Endpoint    string
	TaskName    string
	MainRepo    string
	TaskTarget  string
	Corpus      string
	Timeout     time.Duration
	Jobs        int
}

var (
	args Args

	RootCmd = &cobra.Command{
		Use:   "fuzz",
		Short: "Run fuzz tests",
	}

	FetchCmd = &cobra.Command{
		Use:   "fetch",
		Short: "Fetch submits",
		RunE: func(cmd *cobra.Command, _args []string) error {
			_, _ = cmd, _args
			_, err := FetchSubmits(&args)
			return err
		},
	}

	BuildCmd = &cobra.Command{
		Use:   "build",
		Short: "Build submits",
		RunE: func(cmd *cobra.Command, _args []string) error {
			_, _ = cmd, _args
			_, err := BuildSubmits(&args)
			return err
		},
	}

	RunCmd = &cobra.Command{
		Use:   "run",
		Short: "Fetch, build & run submits",
		RunE: func(cmd *cobra.Command, _args []string) error {
			_, _ = cmd, _args
			return RunFuzzing(&args)
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
	RootCmd.PersistentFlags().BoolVar(&args.NoCache, "no-cache", false, "Do not cache submits / repos")
	RootCmd.PersistentFlags().StringVar(&args.Endpoint, "endpoint", "https://cpp-hse.net", "Scoring system endpoint")
	RootCmd.PersistentFlags().StringVar(&args.GitlabGroup, "gitlab-group", "cpp-advanced-hse-2022", "Gitlab group name")
	RootCmd.PersistentFlags().StringVar(&args.TaskName, "task", "", "Task name")
	RootCmd.PersistentFlags().StringVar(&args.TaskTarget, "target", "", "Build target")
	RootCmd.PersistentFlags().StringVar(&args.Corpus, "corpus", "corpus", "Path to the corpus")
	RootCmd.PersistentFlags().DurationVar(&args.Timeout, "timeout", time.Hour*24, "Fuzzing timeout")
	RootCmd.PersistentFlags().IntVar(&args.Jobs, "jobs", 4, "Number of fuzzing jobs per solution")

	RootCmd.AddCommand(FetchCmd)
	RootCmd.AddCommand(BuildCmd)
	RootCmd.AddCommand(RunCmd)
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
