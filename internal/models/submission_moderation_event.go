package models

import "time"

type SubmissionModerationAction string

const (
	SubmissionModerationActionBan   SubmissionModerationAction = "ban"
	SubmissionModerationActionUnban SubmissionModerationAction = "unban"
)

// SubmissionModerationEvent is an immutable audit record for an actual
// moderation state transition. Current state remains in SubmissionBan for
// fast scorer and admin queries.
type SubmissionModerationEvent struct {
	ID             uint                       `gorm:"primaryKey"`
	PipelineID     int                        `gorm:"index"`
	AdminUserID    uint                       `gorm:"index"`
	Action         SubmissionModerationAction `gorm:"index"`
	Reason         string
	PreviousBanned bool
	CurrentBanned  bool
	CreatedAt      time.Time `gorm:"index"`
}
