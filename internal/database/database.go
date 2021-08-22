package database

import (
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bigredeye/notmanytask/internal/models"
)

type DataBase struct {
	*gorm.DB
}

func OpenDataBase(dsn string) (*DataBase, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&models.User{}, &models.Pipeline{})
	if err != nil {
		return nil, err
	}

	return &DataBase{db}, nil
}

func (db *DataBase) AddUser(user *models.User) error {
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(user).Error
}

func (db *DataBase) GetUserByID(id int) (*models.User, error) {
	var user models.User
	err := db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) GetUserByLogin(login string) (*models.User, error) {
	var user models.User
	err := db.First(&user, "login = ?", login).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) ListUsersWithoutRepos() ([]*models.User, error) {
	var users []*models.User
	err := db.Find(&users, "repository IS NULL").Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (db *DataBase) SetUserRepository(user *models.User) error {
	return db.Model(user).Update("repository", user.Repository).Error
}

func (db *DataBase) AddPipeline(pipeline *models.Pipeline) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status"}),
	}).Create(pipeline).Error
}

func (db *DataBase) ListUserPipelines(login string) (pipelines []models.Pipeline, err error) {
	pipelines = make([]models.Pipeline, 0)
	err = db.Find(&pipelines, "login = ?", login).Error
	if err != nil {
		pipelines = nil
	}
	return
}

func (db *DataBase) ListAllPipelines() (pipelines []models.Pipeline, err error) {
	pipelines = make([]models.Pipeline, 0)
	err = db.Find(&pipelines).Error
	if err != nil {
		pipelines = nil
	}
	return
}
