package models

import "time"

// SubmissionBan records an administrative decision to exclude one GitLab
// pipeline from scoring. It is deliberately separate from Pipeline.Status:
// the GitLab synchronizer refreshes that status and must not undo moderation.
type SubmissionBan struct {
	PipelineID  int `gorm:"primaryKey"`
	AdminUserID uint
	Reason      string
	CreatedAt   time.Time
}
