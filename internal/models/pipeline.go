package models

import (
	"time"
)

type Pipeline struct {
	ID       uint   `gorm:"primaryKey"`
	Task     string `gorm:"index"`
	Login    string `gorm:"index"`
	Status   string
	OpenedAt time.Time
}
