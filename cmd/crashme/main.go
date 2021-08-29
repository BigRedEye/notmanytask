package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sync/semaphore"
)

func main() {
	listener, err := net.Listen("tcp", "localhost:3333")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	sema := semaphore.NewWeighted(8)

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		go handleConnection(conn, sema)
	}
}

const MAX_INPUT_SIZE = 10 * 1024 * 1024 // 10MiB

func handleConnection(conn net.Conn, sema *semaphore.Weighted) {
	defer conn.Close()
	fmt.Printf("New connection from %s\n", conn.RemoteAddr().String())

	err := sema.Acquire(context.Background(), 1)
	if err != nil {
		fmt.Printf("Failed to acquire semaphore: %+v", err)
		return
	}
	defer sema.Release(1)

	buf := make([]byte, MAX_INPUT_SIZE)
	len, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("Failed to read: %+v", err)
		return
	}
	if len == MAX_INPUT_SIZE {
		conn.Write([]byte("Too large input (max 10MiB)"))
		return
	}
	buf = buf[:len]

	pos := strings.IndexByte(string(buf), '\n')
	if pos == -1 {
		conn.Write([]byte("No task name found"))
		return
	}

	task, input := buf[:pos], buf[pos:]
	fmt.Printf("Task %s\n", string(task))

	file, err := os.Create("input.txt")
	if err != nil {
		fmt.Printf("Failed create file: %+v", err)
		return
	}
	file.Write(input)
	file.Close()

	file, err = os.Open("input.txt")
	if err != nil {
		fmt.Printf("Failed open input file: %+v", err)
		return
	}

	fields := strings.Fields(string(task))
	cmd := exec.Command(fields[0], fields[1:]...)
	cmd.Stdin = file

	output, err := cmd.CombinedOutput()
	if err != nil {
		conn.Write([]byte("Command failed:\n"))
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			fmt.Printf("Command %s failed with code %d\n", task, exitError.ExitCode())
			conn.Write(exitError.Stderr)
			conn.Write(output)
		} else {
			fmt.Printf("Command %s failed: %+v\n", task, err)
			conn.Write([]byte("Unknown error\n"))
		}
		return
	}

	conn.Write([]byte("Command finished normally,\nstdin:\t"))
	conn.Write(output)
}
