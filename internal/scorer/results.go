package scorer

import (
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

const (
	TaskStatusAssigned = "assigned"
	TaskStatusFailed   = "failed"
	TaskStatusChecking = "checking"
	TaskStatusSuccess  = "success"
)

type TaskStatus = string

func ClassifyPipelineStatus(status models.PipelineStatus) TaskStatus {
	switch status {
	case models.PipelineStatusFailed:
		return TaskStatusFailed
	case models.PipelineStatusPending:
		return TaskStatusChecking
	case models.PipelineStatusRunning:
		return TaskStatusChecking
	case models.PipelineStatusSuccess:
		return TaskStatusSuccess
	default:
		return TaskStatusAssigned
	}
}

type ScoredTask struct {
	Task     string
	Status   TaskStatus
	Score    int
	MaxScore int

	TaskUrl     string
	PipelineUrl string
}

type ScoredTaskGroup struct {
	Title       string
	PrettyTitle string
	Deadline    deadlines.Date
	Tasks       []ScoredTask

	Score    int
	MaxScore int
}

type User struct {
	FirstName     string
	LastName      string
	GitlabLogin   string
	GitlabProject string
}

func (u User) FullName() string {
	return u.FirstName + " " + u.LastName
}

type UserScores struct {
	Groups   []ScoredTaskGroup
	Score    int
	MaxScore int

	User User
}

type Standings struct {
	Deadlines *deadlines.Deadlines
	Users     []*UserScores
}
