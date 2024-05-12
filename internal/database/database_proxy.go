package database

import (
	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/pkg/errors"
)

type DataBaseProxy struct {
	*DataBase
	Conf *config.Config
}

func (db *DataBaseProxy) UserID(user *models.User) int {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return *user.GitlabID
	case config.GiteaMode:
		return int(*user.GiteaID)
	default:
		return -1
	}
}

func (db *DataBaseProxy) UserLogin(user *models.User) *string {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return user.GitlabLogin
	case config.GiteaMode:
		return user.GiteaLogin
	default:
		return nil
	}
}

func (db *DataBaseProxy) FindUserByLogin(login string) (*models.User, error) {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return db.FindUserByGitlabLogin(login)
	case config.GiteaMode:
		return db.FindUserByGiteaLogin(login)
	default:
		return nil, errors.New("Unknown platform mode")
	}
}

func (db *DataBaseProxy) FindUserByID(id int) (*models.User, error) {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return db.FindUserByGitlabID(id)
	case config.GiteaMode:
		return db.FindUserByGiteaID(int64(id))
	default:
		return nil, errors.New("Unknown platform mode")
	}
}

func (db *DataBaseProxy) ListUsersWithoutRepos() ([]*models.User, error) {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return db.ListGitlabUsersWithoutRepos()
	case config.GiteaMode:
		return db.ListGiteaUsersWithoutRepos()
	default:
		return nil, errors.New("Unknown platform mode")
	}
}

func (db *DataBaseProxy) ListUserFlags(login string) (flags []models.Flag, err error) {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return db.ListUserFlagsGitlab(login)
	case config.GiteaMode:
		return db.ListUserFlagsGitea(login)
	default:
		return nil, errors.New("Unknown platform mode")
	}
}

func (db *DataBaseProxy) ListUserOverrides(login string) (overrides []models.OverriddenScore, err error) {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return db.ListUserOverridesGitlab(login)
	case config.GiteaMode:
		return db.ListUserOverridesGitea(login)
	default:
		return nil, errors.New("Unknown platform mode")
	}
}

func (db *DataBaseProxy) AddOverride(login, task string, score int, status models.PipelineStatus) error {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return db.AddOverrideGitlab(login, task, score, status)
	case config.GiteaMode:
		return db.AddOverrideGitea(login, task, score, status)
	default:
		return nil
	}
}

func (db *DataBaseProxy) RemoveOverride(login, task string) error {
	switch db.Conf.Platform.Mode {
	case config.GitlabMode:
		return db.RemoveOverrideGitlab(login, task)
	case config.GiteaMode:
		return db.RemoveOverrideGitea(login, task)
	default:
		return nil
	}
}
