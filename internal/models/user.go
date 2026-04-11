package models

import (
	"path"

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

	FirstName  string `gorm:"uniqueIndex:idx_name"`
	LastName   string `gorm:"uniqueIndex:idx_name"`
	GroupName  string `gorm:"uniqueIndex:idx_name"`
	TelegramID *int64
	HasRetake  bool
}

// GetProjectName returns the project name extracted from Repository URL.
// Returns empty string if Repository is not set.
func (u *User) GetProjectName() string {
	if u.Repository == nil {
		return ""
	}
	return path.Base(*u.Repository)
}

type Session struct {
	ID     uint   `gorm:"primaryKey"`
	Token  string `gorm:"uniqueIndex"`
	UserID uint
}
