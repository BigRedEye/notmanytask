package deadlines

import (
	"strings"
	"sync"
	"time"
)

type Date struct {
	time.Time
}

// FIXME(BigRedEye): Do not hardcode Moscow

const (
	defaultTimeZone = "Europe/Moscow"
	dateFormat      = "02-01-2006 15:04"
)

var defaultLoc *time.Location
var defaultLocOnce sync.Once

func getDefaultLocation() *time.Location {
	defaultLocOnce.Do(func() {
		var err error
		defaultLoc, err = time.LoadLocation(defaultTimeZone)
		if err != nil {
			panic(err)
		}
	})

	return defaultLoc
}

func (t *Date) UnmarshalText(buf []byte) error {
	tt, err := time.ParseInLocation(dateFormat, strings.TrimSpace(string(buf)), getDefaultLocation())
	if err != nil {
		return err
	}
	t.Time = tt
	return nil
}

func (t Date) MarshalText() ([]byte, error) {
	return []byte(t.Time.Format(dateFormat)), nil
}

type Task struct {
	Task  string
	Score int
}

type TaskGroup struct {
	Group string

	Deadline Date
	Start    Date

	Tasks []Task
}

type Deadlines = []TaskGroup
