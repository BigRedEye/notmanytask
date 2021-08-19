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
const defaultTimeZone = "Europe/Moscow"

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

func (t *Date) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var buf string
	err := unmarshal(&buf)
	if err != nil {
		return nil
	}

	tt, err := time.ParseInLocation("02-01-2006 15:04", strings.TrimSpace(buf), getDefaultLocation())
	if err != nil {
		return err
	}
	t.Time = tt
	return nil
}

func (t Date) MarshalYAML() (interface{}, error) {
	return t.Time.Format("02-01-2006 15:04"), nil
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
