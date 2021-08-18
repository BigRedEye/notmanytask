package deadlines

import (
	"strings"
	"time"
)

type Date struct {
	time.Time
}

func (t *Date) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var buf string
	err := unmarshal(&buf)
	if err != nil {
		return nil
	}

	tt, err := time.Parse("02-01-2006 15:04", strings.TrimSpace(buf))
	if err != nil {
		return err
	}
	t.Time = tt
	return nil
}

func (t Date) MarshalYAML() (interface{}, error) {
	return t.Time.Format("02-01-2006 15:04"), nil
}

type Deadlines = []struct {
	Group string

	Deadline Date
	Start    Date

	Tasks []struct {
		Task  string
		Score int
	}
}
