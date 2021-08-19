package deadlines

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
)

const smallConcurrencyYaml = `
- group:    Intro
  start:    01-01-2021 00:00
  deadline: 28-02-2021 23:59
  tasks:
    - task: intro/aplusb
      score: 0
    - task: intro/deadlock
      score: 100
    - task: intro/dining
      score: 100
    - task: intro/jump
      score: 100
    - task: intro/guarded
      score: 100

- group:    Multi-Threaded Fibers - Intro
  start:    01-01-2021 00:00
  deadline: 20-03-2021 23:59
  tasks:
    - task: fibers/coroutine
      score: 500

- group:    Lock-free
  start:    01-01-2021 00:00
  deadline: 08-06-2021 23:59
  tasks:
    - task: lockfree/stack
      score: 300
    - task: lockfree/shared_ptr
      score: 300
`

func TestDeadlinesParsing(t *testing.T) {
	deadlines := Deadlines{}
	err := yaml.Unmarshal([]byte(smallConcurrencyYaml), &deadlines)
	if err != nil {
		t.Fatal("Failed to parse deadlines:", err)
	}

	expected := Deadlines{{
		Group:    "Intro",
		Start:    Date{time.Date(2021, 1, 1, 0, 0, 0, 0, getDefaultLocation())},
		Deadline: Date{time.Date(2021, 2, 28, 23, 59, 0, 0, getDefaultLocation())},
		Tasks: []Task{{
			Task:  "intro/aplusb",
			Score: 0,
		}, {
			Task:  "intro/deadlock",
			Score: 100,
		}, {
			Task:  "intro/dining",
			Score: 100,
		}, {
			Task:  "intro/jump",
			Score: 100,
		}, {
			Task:  "intro/guarded",
			Score: 100,
		}},
	}, {
		Group:    "Multi-Threaded Fibers - Intro",
		Start:    Date{time.Date(2021, 1, 1, 0, 0, 0, 0, getDefaultLocation())},
		Deadline: Date{time.Date(2021, 3, 20, 23, 59, 0, 0, getDefaultLocation())},
		Tasks: []Task{{
			Task:  "fibers/coroutine",
			Score: 500,
		}},
	}, {
		Group:    "Lock-free",
		Start:    Date{time.Date(2021, 1, 1, 0, 0, 0, 0, getDefaultLocation())},
		Deadline: Date{time.Date(2021, 6, 8, 23, 59, 0, 0, getDefaultLocation())},
		Tasks: []Task{{
			Task:  "lockfree/stack",
			Score: 300,
		}, {
			Task:  "lockfree/shared_ptr",
			Score: 300,
		}},
	}}

	fmt.Printf("%+v\n%+v\n", deadlines, expected)

	eq := cmp.Equal(deadlines, expected)
	if !eq {
		t.Fatal("Not equal")
	}
}
