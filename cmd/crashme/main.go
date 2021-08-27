package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

func main() {
	listener, err := net.Listen("tcp", "localhost:3333")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 1024*1024)
	len, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("Failed to read: %+v", err)
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

	cmd := exec.Command(string(task))
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
