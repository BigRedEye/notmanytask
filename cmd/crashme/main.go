package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/sync/semaphore"

	"github.com/bigredeye/notmanytask/api"
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
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
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
		return "", backoff.Permanent(fmt.Errorf("server error: %s", response.Error))
	}
	return response.Flag, nil
}

func (f flagFetcher) fetchFlag(task string) (string, error) {
	flag := ""

	backoffPolicy := backoff.NewExponentialBackOff()
	backoffPolicy.MaxElapsedTime = time.Second * 10
	err := backoff.Retry(func() error {
		var err error
		flag, err = f.doFetchFlag(task)
		log.Printf("Failed to fetch flag: %+v\n", err)
		return err
	}, backoffPolicy)

	return flag, err
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
	defer func() { _ = listener.Close() }()

	acceptErrorsBudget := 10
	currentAcceptErrorsBudget := acceptErrorsBudget
	connID := 0

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

		go checker.handleConnection(context.Background(), conn, connID)
		connID++
	}
}

// FIXME(BigRedEye): Read from config
const MaxInputSize = 10 * 1024 * 1024 // 10MiB
const MaxFirstLineSizeE = 100

type checker struct {
	binariesDirectory string
	submitsDirectory  string
	sema              *semaphore.Weighted
	flagFetcher       flagFetcher
}

func newChecker(binariesDirectory, submitsDirectory string, concurrencyLevel int64, url, token string) (*checker, error) {
	err := os.MkdirAll(submitsDirectory, 0750)
	if err != nil {
		return nil, fmt.Errorf("failed to mkdir submits directory: %w", err)
	}

	if !isDirectory(binariesDirectory) {
		return nil, fmt.Errorf("binaries directory does not exist")
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
		_ = conn.Close()
		log.Printf("Closed connection #%d from %s\n", connID, conn.RemoteAddr())
	}()
	log.Printf("New connection from #%d from %s\n", connID, conn.RemoteAddr())

	err := c.doHandleConnection(ctx, conn)
	if err != nil {
		log.Printf("Failed to handle connection: %+v", err)
		fmt.Fprintf(conn, "Error: %s\n", err.Error())
	}
}

func slowReadFirstLine(reader io.Reader) (string, error) {
	var str strings.Builder
	buf := []byte{' '}
	for buf[0] != '\n' {
		n, err := reader.Read(buf)

		if err == io.EOF || (err == nil && n != 1) {
			if str.Len() == MaxFirstLineSizeE {
				return "", fmt.Errorf("too long first line")
			}
			return "", fmt.Errorf("got EOF before new line")
		}
		if err != nil {
			return "", err
		}

		str.WriteByte(buf[0])
	}
	return strings.TrimSpace(str.String()), nil
}

func writeStringOrIgnore(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
}

func (c *checker) doHandleConnection(ctx context.Context, conn net.Conn) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute) // FIXME(BigRedEye): Timeout from config
	defer cancel()

	if !c.sema.TryAcquire(1) {
		writeStringOrIgnore(conn, "Waiting for an available runner...\n")
		err := c.sema.Acquire(ctx, 1)
		if err != nil {
			return fmt.Errorf("failed to acquire semaphore: %w", err)
		}
	}
	defer c.sema.Release(1)

	writeStringOrIgnore(conn, "Enter task name: ")
	reader := io.LimitReader(conn, MaxInputSize)
	// User should pass task name in the first line
	task, err := slowReadFirstLine(io.LimitReader(reader, MaxFirstLineSizeE))
	if err != nil {
		return fmt.Errorf("failed to read first line: %w", err)
	}

	fullTaskName := strings.ReplaceAll(task, "_", "-")
	lastTaskName := filepath.Base(fullTaskName)
	executablePath := path.Join(c.binariesDirectory, "ctf_"+strings.ReplaceAll(lastTaskName, "-", "_"))
	if !isRegularFile(executablePath) {
		return fmt.Errorf("unknown task %s", task)
	}

	inputPath := path.Join(c.submitsDirectory, lastTaskName+"_"+time.Now().Format("2006-01-02T15:04:05.000"))
	submitFile, err := os.Create(inputPath)
	if err != nil {
		return fmt.Errorf("failed to create input file: %w", err)
	}
	reader = io.TeeReader(reader, submitFile)

	stderrFile, err := os.Create(inputPath + ".err")
	if err != nil {
		return fmt.Errorf("failed to create stderr file: %w", err)
	}
	stderrBuffer := &bytes.Buffer{}
	stderr := io.MultiWriter(stderrFile, stderrBuffer)

	cmd := exec.Command(executablePath)
	proxy, err := newCommandProxy(reader, conn, stderr, cmd)
	if err != nil {
		return fmt.Errorf("failed to prepare command: %w", err)
	}

	fmt.Fprintf(conn, "Running task %s\n", task)
	err = proxy.run()

	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			log.Printf("Command %s failed with code %d, status: %s", executablePath, exitError.ExitCode(), exitError.String())
			status := exitError.Sys().(syscall.WaitStatus)
			if status.Signal() == os.Interrupt {
				log.Printf("Command was interrupted")
				return fmt.Errorf("got EOF before command exit")
			}

			_, err = fmt.Fprintf(conn, "Command failed: %s\nTrying to fetch flag...\n", exitError.ProcessState)
			if err != nil {
				return fmt.Errorf("failed to write to the connection: %w", err)
			}

			flag, err := c.flagFetcher.fetchFlag(task)
			if err != nil {
				log.Printf("Failed to fetch flag for failed task: %+v\n", err)
				return fmt.Errorf("failed to fetch flag: %+v", err)
			}

			writeStringOrIgnore(conn, flag+"\n")
			return nil
		} else {
			log.Printf("Failed to run command %s: %s", executablePath, stderrBuffer)
			return fmt.Errorf("failed to start command: %w, stderr: %s", err, stderrBuffer)
		}
	}

	writeStringOrIgnore(conn, "Command finished normally\n")
	return nil
}

type commandProxy struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	cmd    *exec.Cmd

	stdinPipe  io.WriteCloser
	stderrPipe io.ReadCloser
	stdoutPipe io.ReadCloser
	wg         *sync.WaitGroup
}

func newCommandProxy(stdin io.Reader, stdout, stderr io.Writer, cmd *exec.Cmd) (*commandProxy, error) {
	proxy := &commandProxy{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		cmd:    cmd,
		wg:     &sync.WaitGroup{},
	}
	proxy.wg.Add(2)

	var err error
	proxy.stdoutPipe, err = cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create command stdout: %w", err)
	}
	proxy.stderrPipe, err = cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create command stderr: %w", err)
	}
	proxy.stdinPipe, err = cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create command stdin: %w", err)
	}

	return proxy, nil
}

func (c *commandProxy) run() error {
	err := c.cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	go c.handleStdin()
	go c.handleStdout()
	go c.handleStderr()

	err = c.cmd.Wait()
	c.wg.Wait()
	return err
}

func (c *commandProxy) handleStdin() {
	_, _ = io.Copy(c.stdinPipe, c.stdin)

	// in case of closed connection
	// we should try stop other io goroutines
	_ = c.stdinPipe.Close()

	log.Printf("Done stdin")
}

func (c *commandProxy) handleStdout() {
	copyAndDone(c.stdout, c.stdoutPipe, c.wg)
	log.Printf("Done stdout")
}

func (c *commandProxy) handleStderr() {
	copyAndDone(c.stderr, c.stderrPipe, c.wg)
	log.Printf("Done stderr")
}

func copyAndDone(dst io.Writer, src io.ReadCloser, wg *sync.WaitGroup) {
	_, _ = io.Copy(dst, src)
	_ = src.Close()
	wg.Done()
}
