package models

import "time"

type Flag struct {
	ID          string  `gorm:"primaryKey"`
	Task        string  `gorm:"index"`
	GitlabLogin *string `gorm:"index"`
	CreatedAt   time.Time
}
