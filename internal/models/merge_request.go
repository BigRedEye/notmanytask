package models

import (
	"time"
)

const (
	MergeRequestStatusFailed  = "failed"
	MergeRequestStatusOk = "ok"
)

type MergeRequestStatus = string

type MergeRequest struct {
	ID      int    `gorm:"primaryKey"`
	Project string `gorm:"index"`

	Task      string `gorm:"index"`
	Status    MergeRequestStatus
}
