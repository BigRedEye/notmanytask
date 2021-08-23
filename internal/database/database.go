package database

import (
	"github.com/google/uuid"
	"github.com/pkg/errors"
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

	err = db.AutoMigrate(&models.User{}, &models.Pipeline{}, &models.Session{})
	if err != nil {
		return nil, err
	}

	return &DataBase{db}, nil
}

func (db *DataBase) AddUser(user *models.User) (*models.User, error) {
	return user, db.Clauses(clause.OnConflict{DoNothing: true}).Create(user).Error
}

func (db *DataBase) FindUserByID(id uint) (*models.User, error) {
	var user models.User
	err := db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) FindUserByGitlabLogin(login string) (*models.User, error) {
	var user models.User
	err := db.First(&user, "gitlab_login = ?", login).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) FindUserByGitlabID(id int) (*models.User, error) {
	var user models.User
	err := db.First(&user, "gitlab_id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (db *DataBase) ListUsersWithoutRepos() ([]*models.User, error) {
	var users []*models.User
	err := db.Find(&users, "repository IS NULL AND gitlab_id IS NOT NULL AND gitlab_login IS NOT NULL").Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (db *DataBase) SetUserGitlabAccount(uid uint, user *models.GitlabUser) error {
	res := db.Model(&models.User{}).
		Where("id = ? AND (gitlab_id IS NULL OR gitlab_login IS NULL)", uid).
		Updates(map[string]interface{}{
			"gitlab_id":    user.GitlabID,
			"gitlab_login": user.GitlabLogin,
		})

	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < 1 {
		return errors.Errorf("Unknown user %d", uid)
	}
	return nil
}

func (db *DataBase) SetUserRepository(user *models.User) error {
	res := db.Model(user).Update("repository", user.Repository)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected < 1 {
		return errors.Errorf("Unknown user %d", user.ID)
	}
	return nil
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

func (db *DataBase) CreateSession(user uint) (*models.Session, error) {
	session := &models.Session{
		Token:  uuid.Must(uuid.NewUUID()).String(),
		UserID: user,
	}
	res := db.DB.Create(session)
	if res.Error != nil {
		return nil, res.Error
	}
	return session, nil
}

func (db *DataBase) FindSession(token string) (*models.Session, error) {
	var session models.Session
	res := db.DB.Where("token", token).Take(&session)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected < 1 {
		return nil, errors.New("Unknown session")
	}
	return &session, nil
}

func (db *DataBase) FindUserBySession(token string) (*models.User, *models.Session, error) {
	session, err := db.FindSession(token)
	if err != nil {
		return nil, nil, err
	}
	user, err := db.FindUserByID(session.UserID)
	if err != nil {
		return nil, session, err
	}
	return user, session, nil
}
