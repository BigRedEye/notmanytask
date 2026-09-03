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

	// Merge request workflow only
	TaskStatusPending        = "pending"         // waiting for the review period to pass
	TaskStatusOnReview       = "on_review"       // has unresolved review notes
	TaskStatusReviewResolved = "review_resolved" // review notes resolved, waiting for merge
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
	// Message is a short human-readable status detail (merge request workflow)
	Message string
	// HasReview is true if a human reviewed the merge request
	HasReview bool

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

type User struct {
	FirstName     string
	LastName      string
	Email         string
	Group         string
	Subgroup      string
	GitlabLogin   string
	GitlabProject string
}

func (u User) FullName() string {
	return u.FirstName + " " + u.LastName
}

type UserScores struct {
	Groups    []ScoredTaskGroup
	Score     int
	MaxScore  int
	FinalMark float64

	User User
}

type Standings struct {
	Deadlines *deadlines.Deadlines
	Users     []*UserScores
}
