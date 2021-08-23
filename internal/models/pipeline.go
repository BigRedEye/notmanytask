package models

import (
	"time"
)

type Pipeline struct {
	ID      int    `gorm:"primaryKey"`
	Project string `gorm:"index"`

	Task      string `gorm:"index"`
	Status    string
	CreatedAt time.Time
}
