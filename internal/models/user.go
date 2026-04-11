package models

import (
	"gorm.io/gorm"
)

type GitlabUser struct {
	GitlabID    *int    `gorm:"uniqueIndex"`
	GitlabLogin *string `gorm:"uniqueIndex"`
	Repository  *string
	ProjectName *string
}

type User struct {
	gorm.Model

	GitlabUser

	FirstName    string `gorm:"uniqueIndex:idx_name"`
	LastName     string `gorm:"uniqueIndex:idx_name"`
	GroupName    string `gorm:"uniqueIndex:idx_name"`
	SubgroupName string `gorm:"uniqueIndex:idx_name"`
	Email        string
}

type Session struct {
	ID     uint   `gorm:"primaryKey"`
	Token  string `gorm:"uniqueIndex"`
	UserID uint
}
