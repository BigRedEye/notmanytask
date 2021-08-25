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
}

type ScoredTaskGroup struct {
	Title    string
	Deadline deadlines.Date
	Tasks    []ScoredTask

	Score    int
	MaxScore int
}

type UserScores struct {
	Groups []ScoredTaskGroup
}
