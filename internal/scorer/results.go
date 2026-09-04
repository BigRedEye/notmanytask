package scorer

import (
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

const (
	TaskStatusAssigned = "assigned"
	TaskStatusBanned   = "banned"
	TaskStatusFailed   = "failed"
	TaskStatusChecking = "checking"
	TaskStatusSuccess  = "success"
)

type TaskStatus = string

func ClassifyPipelineStatus(status models.PipelineStatus) TaskStatus {
	switch status {
	case models.PipelineStatusBanned:
		return TaskStatusBanned
	case models.PipelineStatusFailed:
		return TaskStatusFailed
	case models.PipelineStatusCanceled:
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
	Task      string
	ShortName string

	Status     TaskStatus
	Score      int
	MaxScore   int
	Overridden bool

	TaskUrl     string
	PipelineUrl string
	BranchUrl   string
}

type ScoredTaskGroup struct {
	Title       string
	PrettyTitle string
	Deadline    deadlines.Date
	Tasks       []ScoredTask

	Score    int
	MaxScore int
}

type ScoredScoringGroup struct {
	Name          string
	Score         int
	MaxScore      int
	Weight        float64
	WeightedScore float64
	TenScaleScore float64
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
	Groups        []ScoredTaskGroup
	ScoringGroups []ScoredScoringGroup
	Score         int
	MaxScore      int
	FinalMark     float64
	MaxFinalMark  float64

	User User
}

type Standings struct {
	Deadlines *deadlines.Deadlines
	Users     []*UserScores
}
