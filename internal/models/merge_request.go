package models

import "time"

const (
	MergeRequestStateOpened = "opened"
	MergeRequestStateClosed = "closed"
	MergeRequestStateMerged = "merged"

	MergeRequestStatusCanBeMerged    = "can_be_merged"
	MergeRequestStatusCannotBeMerged = "cannot_be_merged"
)

type MergeRequest struct {
	ID      int    `gorm:"primaryKey"`
	Project string `gorm:"index:idx_merge_request_project_task"`
	Task    string `gorm:"index:idx_merge_request_project_task"`

	State                 string
	UserNotesCount        int
	StartedAt             time.Time
	MergeStatus           string
	IID                   int
	SHA                   string
	MergeUserLogin        string
	HasUnresolvedNotes    bool
	LastNoteCreatedAt     time.Time
	LastPipelineStatus    PipelineStatus
	LastPipelineCreatedAt time.Time
	ExtraChanges          bool
}
