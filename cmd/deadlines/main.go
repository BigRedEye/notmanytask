package main

import (
	"fmt"

	"github.com/bigredeye/notmanytask/internal/deadlines"
)

var data = `
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

- group:    Mutual Exclusion
  start:    01-01-2021 00:00
  deadline: 02-03-2021 23:59
  tasks:
    - task: mutex/livelock
      score: 200
    - task: mutex/mutex
      score: 200
    - task: mutex/spinlock
      score: 200
    - task: mutex/try-lock
      score: 200

- group:    Condition Variables
  start:    01-01-2021 00:00
  deadline: 09-03-2021 23:59
  tasks:
    - task: condvar/barrier
      score: 200
    - task: condvar/condvar
      score: 200
    - task: condvar/semaphore
      score: 200
    - task: condvar/thread-pool
      score: 400

- group:    Multi-Threaded Fibers - Intro
  start:    01-01-2021 00:00
  deadline: 20-03-2021 23:59
  tasks:
    - task: fibers/coroutine
      score: 500

- group:    Asio
  start:    01-01-2021 00:00
  deadline: 22-03-2021 23:59
  tasks:
    - task: asio/echo
      score: 200

- group:    TinyFibers + Asio
  start:    01-01-2021 00:00
  deadline: 28-03-2021 23:59
  tasks:
    - task: tinyfibers/sleep
      score: 300
    - task: tinyfibers/echo
      score: 500

- group:    Executors
  start:    01-01-2021 00:00
  deadline: 18-04-2021 23:59
  tasks:
    - task: futures/executors
      score: 500

- group:    Futures
  start:    01-01-2021 00:00
  deadline: 25-04-2021 23:59
  tasks:
    - task: futures/futures
      score: 500

- group:    Multi-Threaded Fibers - Mutex + Condvar
  start:    01-01-2021 00:00
  deadline: 02-05-2021 23:59
  tasks:
    - task: fibers/mutex
      score: 400

- group:    Multi-Threaded Fibers - Channels + Select
  start:    01-01-2021 00:00
  deadline: 16-05-2021 23:59
  tasks:
    - task: fibers/channel
      score: 500

- group:    Stackless Coroutines / Gor-routines
  start:    01-01-2021 00:00
  deadline: 23-05-2021 23:59
  tasks:
    - task: stackless/gorroutines
      score: 400

- group:    Stackless Coroutines / Task
  start:    01-01-2021 00:00
  deadline: 30-05-2021 23:59
  tasks:
    - task: stackless/task
      score: 400

- group:    Lock-free
  start:    01-01-2021 00:00
  deadline: 08-06-2021 23:59
  tasks:
    - task: lockfree/stack
      score: 300
    - task: lockfree/shared_ptr
      score: 300

`

func main() {
	d, err := deadlines.Fetch("https://gitlab.com/Lipovsky/concurrency-course/-/raw/master/deadlines/hse.yml")
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v", d)

	/*
		for _, group := range d {
			fmt.Println(group.Group)
			fmt.Println(group.Start)
			fmt.Println(group.Deadline)
			for _, task := range group.Tasks {
				fmt.Println("\t- ", task.Task)
				fmt.Println("\t  ", task.Score)
			}
		}
	*/
}
