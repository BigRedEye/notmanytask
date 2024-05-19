package models

import (
	"gorm.io/gorm"
)

type GitlabUser struct {
	GitlabID    *int    `gorm:"uniqueIndex"`
	GitlabLogin *string `gorm:"uniqueIndex"`
}

type GiteaUser struct {
	GiteaID    *int64  `gorm:"uniqueIndex"`
	GiteaLogin *string `gorm:"uniqueIndex"`
}

type User struct {
	gorm.Model

	GitlabUser
	GiteaUser
	Repository *string

	FirstName  string `gorm:"uniqueIndex:idx_name"`
	LastName   string `gorm:"uniqueIndex:idx_name"`
	GroupName  string `gorm:"uniqueIndex:idx_name"`
	TelegramID *int64
	HasRetake  bool
}

type Session struct {
	ID     uint   `gorm:"primaryKey"`
	Token  string `gorm:"uniqueIndex"`
	UserID uint
}
