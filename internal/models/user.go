package models

import (
	"path"
	"strings"

	"gorm.io/gorm"
)

type GitlabUser struct {
	GitlabID    *int    `gorm:"uniqueIndex"`
	GitlabLogin *string `gorm:"uniqueIndex"`
	Repository  *string
	ProjectName string `gorm:"index"`
}

type User struct {
	gorm.Model

	GitlabUser

	FirstName    string `gorm:"uniqueIndex:idx_name"`
	LastName     string `gorm:"uniqueIndex:idx_name"`
	GroupName    string `gorm:"uniqueIndex:idx_name"`
	SubgroupName string `gorm:"uniqueIndex:idx_name;not null;default:''"`
	Email        string
	TelegramID   *int64
	HasRetake    bool
}

// ProjectNameFromRepository is used only while importing legacy users and when
// a repository is first assigned. Runtime joins use the stored ProjectName.
func ProjectNameFromRepository(repository string) string {
	repository = strings.TrimSpace(strings.TrimRight(repository, "/"))
	if repository == "" {
		return ""
	}
	return path.Base(repository)
}

// GetProjectName returns the stored project identity. The Repository fallback
// keeps unsaved and pre-migration User values compatible.
func (u *User) GetProjectName() string {
	if u.ProjectName != "" {
		return u.ProjectName
	}
	if u.Repository == nil {
		return ""
	}
	return ProjectNameFromRepository(*u.Repository)
}

func (u *User) BeforeSave(_ *gorm.DB) error {
	if u.ProjectName == "" && u.Repository != nil {
		u.ProjectName = ProjectNameFromRepository(*u.Repository)
	}
	return nil
}

type Session struct {
	ID     uint   `gorm:"primaryKey"`
	Token  string `gorm:"uniqueIndex"`
	UserID uint
}
