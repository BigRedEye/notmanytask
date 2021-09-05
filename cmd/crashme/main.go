package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/bigredeye/notmanytask/api"
	"golang.org/x/sync/semaphore"
)

func isRegularFile(path string) bool {
	if stat, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	} else if err != nil {
		log.Printf("Failed to stat file %s: %+v", path, err)
		return false
	} else {
		return stat.Mode().IsRegular()
	}
}

func isDirectory(path string) bool {
	if stat, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	} else if err != nil {
		log.Printf("Failed to stat file %s: %+v", path, err)
		return false
	} else {
		return stat.Mode().IsDir()
	}
}

type flagFetcher struct {
	url   string
	token string
}

func (f flagFetcher) doFetchFlag(task string) (string, error) {
	buf, err := json.Marshal(&api.FlagRequest{
		Token: f.token,
		Task:  task,
	})
	if err != nil {
		return "", err
	}
	res, err := http.Post(f.url, "application/json", bytes.NewReader(buf))
	if err != nil {
		log.Printf("Failed to send flag request: %+v\n", err)
		return "", err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Failed to read response body: %+v\n", err)
		return "", err
	}

	response := api.FlagResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Printf("Failed to parse response json: %+v\n", err)
		return "", err
	}
	if !response.Ok {
		log.Printf("Flag request failed: %s\n", response.Error)
		return "", fmt.Errorf("Server error: %s", response.Error)
	}
	return response.Flag, nil
}

func (f flagFetcher) fetchFlag(task string) (string, error) {
	iterations := 5
	for {
		flag, err := f.doFetchFlag(task)
		if err == nil || iterations <= 0 {
			return flag, err
		}

		log.Printf("Failed to fetch flag: %+v\n", err)
		iterations--
		time.Sleep(time.Second * 5)
	}
}

func main() {
	listenAddress := flag.String("address", ":3333", "Address to listen on")
	binariesDirectory := flag.String("build", "", "Path to build directory")
	submitsDirectory := flag.String("submits", "", "Path to directory to store submits")
	concurrencyLevel := flag.Int64("concurrency", 16, "Max number of computation-heavy tasks to run")
	flag.Parse()

	checker, err := newChecker(*binariesDirectory, *submitsDirectory, *concurrencyLevel, os.Getenv("CRASHME_URL"), os.Getenv("CRASHME_TOKEN"))
	if err != nil {
		panic(err)
	}

	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	acceptErrorsBudget := 10
	currentAcceptErrorsBudget := acceptErrorsBudget
	connId := 0

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept: %+v", err)
			if currentAcceptErrorsBudget == 0 {
				panic(err)
			}
			currentAcceptErrorsBudget--
		} else if currentAcceptErrorsBudget < acceptErrorsBudget {
			currentAcceptErrorsBudget++
		}

		go checker.handleConnection(context.Background(), conn, connId)
		connId++
	}
}

// FIXME(BigRedEye): Read from config
const MAX_INPUT_SIZE = 10 * 1024 * 1024 // 10MiB
const MAX_FIRST_LINE_SIZE = 100

type checker struct {
	binariesDirectory string
	submitsDirectory  string
	sema              *semaphore.Weighted
	flagFetcher       flagFetcher
}

func newChecker(binariesDirectory, submitsDirectory string, concurrencyLevel int64, url, token string) (*checker, error) {
	err := os.MkdirAll(submitsDirectory, 0755)
	if err != nil {
		return nil, fmt.Errorf("Failed to mkdir submits directory: %w", err)
	}

	if !isDirectory(binariesDirectory) {
		return nil, fmt.Errorf("Binaries directory does not exist")
	}

	return &checker{
		binariesDirectory: binariesDirectory,
		submitsDirectory:  submitsDirectory,
		sema:              semaphore.NewWeighted(concurrencyLevel),
		flagFetcher: flagFetcher{
			token: token,
			url:   url,
		},
	}, nil
}

func (c *checker) handleConnection(ctx context.Context, conn net.Conn, connID int) {
	defer func() {
		conn.Close()
		log.Printf("Closed connection #%d from %s\n", connID, conn.RemoteAddr())
	}()
	log.Printf("New connection from #%d from %s\n", connID, conn.RemoteAddr())

	err := c.doHandleConnection(ctx, conn)
	if err != nil {
		log.Printf("Failed to handle connection: %+v", err)
		io.WriteString(conn, fmt.Sprintf("Error: %s", err.Error()))
	}
}

func slowReadFirstLine(reader io.Reader) (string, error) {
	var str strings.Builder
	buf := []byte{' '}
	for buf[0] != '\n' {
		n, err := reader.Read(buf)

		if err == io.EOF || (err == nil && n != 1) {
			if str.Len() == MAX_FIRST_LINE_SIZE {
				return "", fmt.Errorf("Too long first line")
			}
			return "", fmt.Errorf("EOF before new line")
		}
		if err != nil {
			return "", err
		}

		str.WriteByte(buf[0])
	}
	return strings.TrimSpace(str.String()), nil
}

func (c *checker) doHandleConnection(ctx context.Context, conn net.Conn) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute) // FIXME(BigRedEye): Timeout from config
	defer cancel()

	if !c.sema.TryAcquire(1) {
		io.WriteString(conn, "Waiting for an available runner...\n")
		err := c.sema.Acquire(ctx, 1)
		if err != nil {
			return fmt.Errorf("Failed to acquire semaphore: %w", err)
		}
	}
	defer c.sema.Release(1)

	io.WriteString(conn, "Enter task name: ")
	reader := io.LimitReader(conn, MAX_INPUT_SIZE)
	// User should pass task name in the first line
	task, err := slowReadFirstLine(io.LimitReader(reader, MAX_FIRST_LINE_SIZE))
	if err != nil {
		return fmt.Errorf("Failed to read first line: %w", err)
	}

	executablePath := path.Join(c.binariesDirectory, "ctf_"+strings.ReplaceAll(task, "-", "_"))
	if !isRegularFile(executablePath) {
		return fmt.Errorf("Unknown task %s", task)
	}

	inputPath := path.Join(c.submitsDirectory, task+"_"+time.Now().Format("2006-01-02T15:04:05.000"))
	submitFile, err := os.Create(inputPath)
	if err != nil {
		return fmt.Errorf("Failed to create input file: %w", err)
	}

	reader = io.TeeReader(reader, submitFile)
	stderr := bytes.Buffer{}

	cmd := exec.Command(executablePath)
	cmd.Stdin = reader
	cmd.Stdout = conn
	cmd.Stderr = &stderr

	io.WriteString(conn, fmt.Sprintf("Running task %s\n", task))
	err = cmd.Run()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			log.Printf("Command %s failed with code %d\n", task, exitError.ExitCode())
			io.WriteString(conn, fmt.Sprintf("Command failed: %s\n", exitError.ProcessState))

			flag, err := c.flagFetcher.fetchFlag(task)
			if err != nil {
				log.Printf("Failed to fetch flag for failed task: %+v\n", err)
				return err
			}

			io.WriteString(conn, flag)
			return nil
		} else {
			log.Printf("Failed to run command %s: %s", executablePath, stderr)
			return fmt.Errorf("Failed to start command: %w, stderr: %s", err, stderr)
		}
	}

	conn.Write([]byte("Command finished normally"))
	return nil
}
