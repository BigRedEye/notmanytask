package models

import (
	"gorm.io/gorm"
)

type OverriddenScore struct {
	gorm.Model

	GitlabLogin string `gorm:"uniqueIndex:idx_overrides"`
	Task        string `gorm:"uniqueIndex:idx_overrides"`

	Score  int
	Status PipelineStatus
}
