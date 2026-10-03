package models

import "time"

// SubmissionBan excludes one GitLab pipeline from scoring. It is separate
// from Pipeline.Status: the GitLab synchronizer refreshes that status and
// must not undo moderation.
type SubmissionBan struct {
	PipelineID int `gorm:"primaryKey"`
	Reason     string
	CreatedAt  time.Time
}
