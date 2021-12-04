package deadlines

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const smallConcurrencyYaml = `
- title:    Intro
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

- title:    Multi-Threaded Fibers - Intro
  start:    01-01-2021 00:00
  deadline: 20-03-2021 23:59
  tasks:
    - task: fibers/coroutine
      score: 500

- title:    Lock-free
  start:    01-01-2021 00:00
  deadline: 08-06-2021 23:59
  tasks:
    - task: lockfree/stack
      score: 300
    - task: lockfree/shared_ptr
      score: 300
`

func TestDeadlinesParsingV1(t *testing.T) {
	deadlines, err := parseV1([]byte(smallConcurrencyYaml))
	if err != nil {
		t.Fatal("Failed to parse deadlines:", err)
	}

	expected := []TaskGroup{{
		Title:    "Intro",
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
		Title:    "Multi-Threaded Fibers - Intro",
		Start:    Date{time.Date(2021, 1, 1, 0, 0, 0, 0, getDefaultLocation())},
		Deadline: Date{time.Date(2021, 3, 20, 23, 59, 0, 0, getDefaultLocation())},
		Tasks: []Task{{
			Task:  "fibers/coroutine",
			Score: 500,
		}},
	}, {
		Title:    "Lock-free",
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

	fmt.Printf("%+v\n%+v\n", deadlines.Assignments, expected)

	eq := cmp.Equal(deadlines.Assignments, expected)
	if !eq {
		t.Fatal("Not equal")
	}
}

const largeCppYaml = `
# Add new groups at the bottom

scoring:
  policies:
  - name: hard
    kind: linear
    spec:
      after: 5m
      multiplier: 0
  - name: notsohard
    kind: exp
    spec:
      multiplier: 120h
      threshold: 0.3

  groups:
  - name: weekly
    weight: 4.0
    policy: notsohard

  - name: large
    weight: 6.0
    policy: hard

  - name: bonus
    weight: 2.0
    policy: hard

  defaultGroup: weekly

assignments:
- title:     00-setup
  start:    17-07-2021 18:00
  deadline: 21-09-2022 18:00
  tasks:
    - task: multiplication
      score: 100

- title:     00-setup-bonus
  start:    17-07-2021 18:00
  deadline: 21-09-2022 18:00
  tasks:
    - task: gcd
      score: 100

- title:     01-move
  start:    04-09-2021 13:00
  deadline: 17-09-2021 19:00
  tasks:
    - task: dedup
      score: 100
    - task: harakiri
      score: 100
    - task: rule-of-5
      score: 100
    - task: cow-vector
      score: 150
    - task: deque
      score: 250

- title:     02-memory
  start:    11-09-2021 13:00
  deadline: 24-09-2021 19:00
  tasks:
    - task: string-view
      score: 100
    - task: lru-cache
      score: 100
    - task: intrusive-list
      score: 100
    - task: reallol
      score: 100
    - task: string-operations
      score: 150
    - task: compressed_pair
      score: 150

- title:     03-types
  start:    18-09-2021 13:00
  deadline: 08-10-2021 19:00
  tasks:
    - task: dungeon
      score: 100
    - task: fold
      score: 100
    - task: bind_front
      score: 100
    - task: curry
      score: 100
    - task: stdflib
      score: 100

- title:     03-smart-ptrs
  start:    18-09-2021 13:00
  deadline: 10-10-2021 19:00
  group:    large
  tasks:
    - task:  smart-ptrs/unique_basic
      score: 2
    - task:  smart-ptrs/unique_advanced
      score: 2
    - task:  smart-ptrs/shared_basic
      score: 3
    - task:  smart-ptrs/shared_weak
      score: 2
    - task:  smart-ptrs/shared_from_this
      score: 1

- title:     05-errors
  start:    02-10-2021 13:00
  deadline: 15-10-2021 19:00
  tasks:
    - task: safe-transform
      score: 100
    - task: defer
      score: 100
    - task: tryhard
      score: 100
    - task: grep
      score: 100

- title:     06-patterns
  start:    09-10-2021 13:00
  deadline: 29-10-2021 19:00
  tasks:
    - task: pimpl
      score: 100
    - task: any
      score: 100
    - task: editor
      score: 100
    - task: small-test-framework
      score: 100
    - task: scala-vector
      score: 150

- title:     07-meta
  start:    30-10-2021 13:00
  deadline: 12-11-2021 19:00
  tasks:
    - task: algo_spec
      score: 100
    - task: compile-eval
      score: 100
    - task: transform_tuple
      score: 100
    - task: constexpr-map
      score: 100
    - task: pipes
      score: 100

- title:     08-baby-threads
  start:    06-11-2021 13:00
  deadline: 19-11-2021 20:20
  tasks:
    - task: is-prime
      score: 100
    - task: reduce
      score: 100
    - task: hash-table
      score: 100
    - task: subset-sum
      score: 150

- title:     09-condition-variables
  start:    13-11-2021 13:00
  deadline: 26-11-2021 20:20
  tasks:
    - task: timerqueue
      score: 100
    - task: semaphore
      score: 100
    - task: rw-lock
      score: 100
    - task: buffered-channel
      score: 100
    - task: unbuffered-channel
      score: 100

- title:     10-advanced-threads
  start:    21-11-2021 13:00
  deadline: 04-12-2021 19:00
  tasks:
    - task: futex
      score: 100
    - task: fast-queue
      score: 100
    - task: mpsc-stack
      score: 100
    - task: rw-spinlock
      score: 100

- title:     11-threading-ending
  start:    27-11-2021 13:00
  deadline: 10-12-2021 19:00
  tasks:
    - task: coroutine
      score: 100
    - task: generator
      score: 100

- title:     scheme
  start:    26-10-2021 00:00
  deadline: 18-11-2021 00:01
  group:    large
  tasks:
    - task: scheme/tokenizer
      score: 2
    - task: scheme/parser
      score: 2
    - task: scheme/basic
      score: 3
    - task: scheme/advanced
      score: 3

- title:     jpeg-decoder
  start:    30-11-2021 00:00
  deadline: 26-12-2021 23:59
  group:    large
  tasks:
    - task: jpeg-decoder/huffman
      score: 20
    - task: jpeg-decoder/fftw
      score: 15
    - task: jpeg-decoder/baseline
      score: 25
    - task: jpeg-decoder/faster
      score: 15
    - task: jpeg-decoder/fuzzing
      score: 25
    - task: jpeg-decoder/progressive
      score: 25

- title:     bonus
  start:    17-07-2021 18:00
  deadline: 21-09-2022 18:00
  group:    bonus
  tasks:
    - task: bad-hash
      score: 100
    - task: solve_or_die
      score: 100
    - task: brainfuck
      score: 100
    - task: matrix-2.0
      score: 100
    - task: concepts
      score: 100
    - task: simple_defer
      score: 100
    - task: executors
      score: 200
`

func TestDeadlinesParsingV2(t *testing.T) {
	deadlines, err := parseV2([]byte(largeCppYaml))
	if err != nil {
		t.Fatal("Failed to parse deadlines:", err)
	}

	fmt.Printf("%+v\n", deadlines)

	/*
		expected := Deadlines{
			Assignments: []TaskGroup{{
				Title:    "Intro",
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
				Title:    "Multi-Threaded Fibers - Intro",
				Start:    Date{time.Date(2021, 1, 1, 0, 0, 0, 0, getDefaultLocation())},
				Deadline: Date{time.Date(2021, 3, 20, 23, 59, 0, 0, getDefaultLocation())},
				Tasks: []Task{{
					Task:  "fibers/coroutine",
					Score: 500,
				}},
			}, {
				Title:    "Lock-free",
				Start:    Date{time.Date(2021, 1, 1, 0, 0, 0, 0, getDefaultLocation())},
				Deadline: Date{time.Date(2021, 6, 8, 23, 59, 0, 0, getDefaultLocation())},
				Tasks: []Task{{
					Task:  "lockfree/stack",
					Score: 300,
				}, {
					Task:  "lockfree/shared_ptr",
					Score: 300,
				}},
			}},
		}

		fmt.Printf("%+v\n%+v\n", deadlines.Assignments, expected)

		eq := cmp.Equal(deadlines.Assignments, expected)
		if !eq {
			t.Fatal("Not equal")
		}
	*/
}
