package scorer

import (
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

const (
	TaskStatusAssigned = iota
	TaskStatusFailed
	TaskStatusChecking
	TaskStatusSuccess
)

type TaskStatus = int

func classifyPipelineStatus(status models.PipelineStatus) TaskStatus {
	switch status {
	case models.PipelineStatusFailed:
		return TaskStatusFailed
	case models.PipelineStatusPending:
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
	Gropus []ScoredTaskGroup
}
