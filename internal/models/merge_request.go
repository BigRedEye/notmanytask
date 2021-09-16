package models

const (
	MergeRequestStateOpen  = "open"
	MergeRequestStateClosed = "closed"
)

type MergeRequestState = string

type MergeRequest struct {
	ID      int    `gorm:"primaryKey"`
	Project string `gorm:"index"`

	Task      string `gorm:"index"`
	State     MergeRequestState
}
