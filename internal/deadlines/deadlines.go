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
	Task    string
	Score   int
	Crashme bool
}

type TaskGroup struct {
	Title string
	Group string

	Deadline Date
	Start    Date

	Tasks []Task
}

type Deadlines struct {
	Assignments []TaskGroup
	Scoring     Scoring

	policies map[string]ScoringPolicy `yaml:"-"`
	groups   map[string]*ScoringGroup `yaml:"-"`
}

func (d *Deadlines) buildScoringGroups() error {
	d.policies = make(map[string]ScoringPolicy)
	for i := range d.Scoring.Policies {
		policy := &d.Scoring.Policies[i]
		d.policies[policy.Name] = policy.Policy
	}
	d.groups = make(map[string]*ScoringGroup)
	for i := range d.Scoring.Groups {
		group := &d.Scoring.Groups[i]
		d.groups[group.Name] = group
	}

	maxScores := make(map[string]int)
	for i := range d.Assignments {
		group := d.GetScoringGroup(&d.Assignments[i])
		if group != nil && group.MaxScore == 0 {
			totalScore := 0
			for j := range d.Assignments[i].Tasks {
				totalScore += d.Assignments[i].Tasks[j].Score
			}
			maxScores[group.Name] += totalScore
		}
	}

	for k, v := range maxScores {
		d.groups[k].MaxScore = v
	}

	return nil
}

func (d *Deadlines) GetScoringGroup(group *TaskGroup) *ScoringGroup {
	scoringGroupName := group.Group
	if scoringGroupName == "" {
		scoringGroupName = d.Scoring.DefaultGroup
	}

	g, found := d.groups[scoringGroupName]
	if !found {
		return nil
	}
	return g
}

func (d *Deadlines) GetScoringPolicy(group *TaskGroup) ScoringPolicy {
	scoringGroup := d.GetScoringGroup(group)
	if scoringGroup == nil {
		return nil
	}

	policy, found := d.policies[scoringGroup.Policy]
	if !found {
		return nil
	}
	return policy
}

func (d *Deadlines) HasTask(name string) bool {
	for _, assignment := range d.Assignments {
		for _, task := range assignment.Tasks {
			if task.Task == name {
				return true
			}
		}
	}
	return false
}
