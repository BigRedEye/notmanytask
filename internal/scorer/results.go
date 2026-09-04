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

	// Merge request workflow only. The status is derived from the merge
	// request that represents the task (see mergeRequestBetter: the highest
	// row of this table among the task's requests wins; among merged ones a
	// creditable request with the earliest pipeline). Nothing is stored, a
	// sync recomputes everything.
	//
	//   merge request                              status             score
	//   merged, last pipeline green                success            by pipeline time
	//   merged, last pipeline not green (re-check) failed             0
	//   open, last pipeline failed                 failed             0
	//   open, touches files outside tasks/<task>/  failed             0
	//   open, gitlab says conflict                 failed             0
	//   open, unresolved review threads            review_unresolved  0
	//   open, threads resolved, waiting            review_resolved    0
	//   open, green, waiting for the review period review_pending     0
	//   closed                                     review_pending     0
	//   no merge request                           assigned           0
	//
	// Flags (crashme) and overrides apply on top, as in the pipeline workflow.
	TaskStatusReviewPending    = "review_pending"    // green, waiting for the review period to pass
	TaskStatusReviewUnresolved = "review_unresolved" // has unresolved review threads
	TaskStatusReviewResolved   = "review_resolved"   // review threads resolved, waiting for merge
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
