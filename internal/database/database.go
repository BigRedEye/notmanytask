package database

import (
	"github.com/bigredeye/notmanytask/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DataBase struct {
	db *gorm.DB
}

func OpenDataBase(dsn string) (*DataBase, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&models.Student{}, &models.Pipeline{})
	if err != nil {
		return nil, err
	}

	return &DataBase{db}, nil
}
