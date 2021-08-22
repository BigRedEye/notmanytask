package gitlab

import (
	"context"
	"time"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/models"
	"go.uber.org/zap"
)

type ProjectsMaker struct {
	*Client

	db    *database.DataBase
	users chan *models.User
}

func NewProjectsMaker(client *Client, db *database.DataBase) (*ProjectsMaker, error) {
	return &ProjectsMaker{client, db, make(chan *models.User, 10)}, nil
}

func (p ProjectsMaker) Run(ctx context.Context) {
	p.initializeMissingProjects()

	tick := time.Tick(time.Second * 10)
	select {
	case user := <-p.users:
		p.logger.Info("Got user from in-proc channel", zap.Int("user_id", user.ID), zap.String("user_login", user.Login))
		p.maybeInitializeProject(user)
	case _ = <-tick:
		p.initializeMissingProjects()
	case _ = <-ctx.Done():
		return
	}
}

func (p ProjectsMaker) initializeMissingProjects() {
	users, err := p.db.ListUsersWithoutRepos()
	if err != nil {
		p.logger.Error("Failed to list users without repos", zap.Error(err))
		return
	}

	for _, user := range users {
		p.logger.Info("Got user without repo from database channel", zap.Int("user_id", user.ID), zap.String("user_login", user.Login))
		p.maybeInitializeProject(user)
	}
}

func (p ProjectsMaker) maybeInitializeProject(user *models.User) {
	log := p.logger.With(zap.Int("user_id", user.ID), zap.String("user_login", user.Login))

	err := p.InitializeProject(user)
	if err != nil {
		log.Error("Failed to initialize project", zap.Error(err))
		p.users <- user
		// TODO(BigRedEye): nice backoff
		time.Sleep(time.Millisecond * 100)
		return
	}

	project := p.MakeProjectUrl(user)
	log = log.With(zap.String("project", project))

	user.Repository = &project
	err = p.db.SetUserRepository(user)
	if err != nil {
		log.Error("Failed to set user repo", zap.Error(err))
		return
	}

	log.Info("Sucessfully set user repo")
}
