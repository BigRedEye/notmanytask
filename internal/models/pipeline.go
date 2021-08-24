package models

import (
	"time"
)

const (
	PipelineStatusFailed  = "failed"
	PipelineStatusPending = "pending"
	PipelineStatusRunning = "running"
	PipelineStatusSuccess = "success"
)

type PipelineStatus = string

type Pipeline struct {
	ID      int    `gorm:"primaryKey"`
	Project string `gorm:"index"`

	Task      string `gorm:"index"`
	Status    PipelineStatus
	CreatedAt time.Time
}
