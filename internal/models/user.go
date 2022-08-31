package models

import (
	"gorm.io/gorm"
)

type GitlabUser struct {
	GitlabID    *int    `gorm:"uniqueIndex"`
	GitlabLogin *string `gorm:"uniqueIndex"`
	Repository  *string
}

type User struct {
	gorm.Model

	GitlabUser

	FirstName     string `gorm:"uniqueIndex:idx_name"`
	LastName      string `gorm:"uniqueIndex:idx_name"`
	GroupName     string `gorm:"uniqueIndex:idx_name"`
	TelegramLogin string `gorm:"uniqueIndex:telegram"`
	HasRetake     bool
}

type Session struct {
	ID     uint   `gorm:"primaryKey"`
	Token  string `gorm:"uniqueIndex"`
	UserID uint
}
