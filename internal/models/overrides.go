package models

import (
	"gorm.io/gorm"
)

type OverriddenScore struct {
	gorm.Model

	Login string `gorm:"uniqueIndex:idx_overrides"`
	Task  string `gorm:"uniqueIndex:idx_overrides"`

	Score  int
	Status PipelineStatus
}
