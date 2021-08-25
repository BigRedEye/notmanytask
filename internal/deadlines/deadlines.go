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

func (t *Date) String() string {
	return t.Time.Format(dateFormat)
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
	return []byte(t.String()), nil
}

func (t *Date) UnmarshalJSON(buf []byte) error {
	return t.UnmarshalText(buf[1 : len(buf)-2])
}

func (t Date) MarshalJSON() ([]byte, error) {
	text, err := t.MarshalText()
	if err != nil {
		return text, err
	}
	res := make([]byte, 0, len(text)+2)
	res = append(res, '"')
	res = append(res, text...)
	res = append(res, '"')
	return res, nil
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
