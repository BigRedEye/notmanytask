// Command checktestjson verifies that required tests ran and passed in a fresh
// go test -json event stream.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const maxEventBytes = 4 << 20

type testID struct {
	Package string
	Test    string
}

type testState struct {
	ran      bool
	terminal string
}

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

func main() {
	manifestPath := flag.String("manifest", "", "path to the required-test manifest")
	flag.Parse()
	if *manifestPath == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: checktestjson -manifest PATH")
		os.Exit(2)
	}
	if err := check(*manifestPath, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "checktestjson:", err)
		os.Exit(1)
	}
}

func check(manifestPath string, input io.Reader) error {
	required, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	states := make(map[testID]*testState, len(required))
	for _, id := range required {
		states[id] = new(testState)
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxEventBytes)
	eventCount := 0
	for scanner.Scan() {
		eventCount++
		line := scanner.Bytes()
		if len(line) == 0 {
			return fmt.Errorf("event %d is blank", eventCount)
		}
		var event testEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("decode event %d: %w", eventCount, err)
		}
		if event.Action == "" || event.Package == "" {
			return fmt.Errorf("event %d lacks Action or Package", eventCount)
		}
		if !knownAction(event.Action) {
			return fmt.Errorf("event %d has unknown action %q", eventCount, event.Action)
		}
		if event.Action == "fail" {
			if event.Test == "" {
				return fmt.Errorf("package %s failed", event.Package)
			}
			return fmt.Errorf("test %s %s failed", event.Package, event.Test)
		}

		if event.Action == "skip" {
			if owner, ok := requiredOwner(required, event.Package, event.Test); ok {
				return fmt.Errorf("required test %s %s skipped at %s", owner.Package, owner.Test, event.Test)
			}
		}

		id := testID{Package: event.Package, Test: event.Test}
		state, requiredExactly := states[id]
		if !requiredExactly {
			continue
		}
		switch event.Action {
		case "run":
			if state.ran {
				return fmt.Errorf("required test %s %s has duplicate run", id.Package, id.Test)
			}
			if state.terminal != "" {
				return fmt.Errorf("required test %s %s ran after terminal %s", id.Package, id.Test, state.terminal)
			}
			state.ran = true
		case "pass", "skip":
			if state.terminal != "" {
				return fmt.Errorf("required test %s %s has duplicate/conflicting terminals %s and %s", id.Package, id.Test, state.terminal, event.Action)
			}
			if !state.ran {
				return fmt.Errorf("required test %s %s has %s without run", id.Package, id.Test, event.Action)
			}
			state.terminal = event.Action
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read event stream: %w", err)
	}
	if eventCount == 0 {
		return errors.New("event stream is empty")
	}

	for _, id := range required {
		state := states[id]
		if !state.ran {
			return fmt.Errorf("required test %s %s did not run", id.Package, id.Test)
		}
		if state.terminal != "pass" {
			return fmt.Errorf("required test %s %s did not pass", id.Package, id.Test)
		}
	}
	return nil
}

func readManifest(path string) ([]testID, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("manifest is empty")
	}
	if !utf8.Valid(data) {
		return nil, errors.New("manifest is not valid UTF-8")
	}
	if data[len(data)-1] != '\n' {
		return nil, errors.New("manifest must end with one LF")
	}
	text := string(data[:len(data)-1])
	if text == "" {
		return nil, errors.New("manifest has no entries")
	}
	lines := strings.Split(text, "\n")
	required := make([]testID, 0, len(lines))
	previous := ""
	for i, line := range lines {
		lineNumber := i + 1
		if line == "" {
			return nil, fmt.Errorf("manifest line %d is blank", lineNumber)
		}
		if strings.Count(line, "\t") != 1 {
			return nil, fmt.Errorf("manifest line %d must contain exactly one TAB", lineNumber)
		}
		parts := strings.SplitN(line, "\t", 2)
		if !validImportPath(parts[0]) {
			return nil, fmt.Errorf("manifest line %d has invalid full import path %q", lineNumber, parts[0])
		}
		if !validTopLevelTest(parts[1]) {
			return nil, fmt.Errorf("manifest line %d has invalid top-level test name %q", lineNumber, parts[1])
		}
		if i > 0 {
			switch strings.Compare(previous, line) {
			case 0:
				return nil, fmt.Errorf("manifest line %d duplicates the previous entry", lineNumber)
			case 1:
				return nil, fmt.Errorf("manifest line %d is out of byte order", lineNumber)
			}
		}
		previous = line
		required = append(required, testID{Package: parts[0], Test: parts[1]})
	}
	return required, nil
}

func validImportPath(path string) bool {
	if path == "" || !strings.Contains(path, ".") || !strings.Contains(path, "/") {
		return false
	}
	return !strings.ContainsAny(path, " \t\r\n\\") && !strings.HasPrefix(path, "/") && !strings.HasSuffix(path, "/")
}

func validTopLevelTest(name string) bool {
	if !strings.HasPrefix(name, "Test") || len(name) == len("Test") {
		return false
	}
	for _, r := range name[len("Test"):] {
		if r != '_' && (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func knownAction(action string) bool {
	switch action {
	case "start", "run", "pause", "cont", "pass", "bench", "fail", "output", "skip":
		return true
	default:
		return false
	}
}

func requiredOwner(required []testID, pkg, test string) (testID, bool) {
	for _, id := range required {
		if id.Package == pkg && (test == id.Test || strings.HasPrefix(test, id.Test+"/")) {
			return id, true
		}
	}
	return testID{}, false
}
