package models

import (
	"time"
)

const (
	PipelineStatusBanned   = "banned"
	PipelineStatusFailed   = "failed"
	PipelineStatusPending  = "pending"
	PipelineStatusRunning  = "running"
	PipelineStatusSuccess  = "success"
	PipelineStatusCanceled = "canceled"
)

type PipelineStatus = string

type Pipeline struct {
	ID      int    `gorm:"primaryKey"`
	Project string `gorm:"index"`

	ProjectID int
	Ref       string

	Task      string `gorm:"index"`
	Status    PipelineStatus
	StartedAt time.Time
}
