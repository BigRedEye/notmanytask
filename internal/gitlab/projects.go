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
	return &ProjectsMaker{client, db, make(chan *models.User, 4)}, nil
}

func (p ProjectsMaker) AsyncPrepareProject(user *models.User) {
	p.users <- user
}

func (p ProjectsMaker) Run(ctx context.Context) {
	p.initializeMissingProjects()

	tick := time.Tick(time.Second * 10)
	for {
		select {
		case user := <-p.users:
			p.logger.Info("Got user from in-proc channel",
				zap.Int("gitlab_id", user.GitlabID),
				zap.String("gitlab_login", user.GitlabLogin),
			)
			if !p.maybeInitializeProject(user) {
				p.users <- user
			}
		case <-tick:
			p.initializeMissingProjects()
		case <-ctx.Done():
			p.logger.Info("Stopping projects maker")
			return
		}
	}
}

func (p ProjectsMaker) initializeMissingProjects() {
	users, err := p.db.ListUsersWithoutRepos()
	if err != nil {
		p.logger.Error("Failed to list users without repos", zap.Error(err))
		return
	}

	for _, user := range users {
		p.logger.Info("Got user without repo from database channel",
			zap.Int("gitlab_id", user.GitlabID),
			zap.String("gitlab_login", user.GitlabLogin),
		)
		p.maybeInitializeProject(user)
	}
}

func (p ProjectsMaker) maybeInitializeProject(user *models.User) bool {
	log := p.logger.With(zap.Int("gitlab_id", user.GitlabID), zap.String("gitlab_login", user.GitlabLogin))

	err := p.InitializeProject(user)
	if err != nil {
		log.Error("Failed to initialize project", zap.Error(err))
		// TODO(BigRedEye): nice backoff
		time.Sleep(time.Millisecond * 100)
		return false
	}

	project := p.MakeProjectUrl(user)
	log = log.With(zap.String("project", project))

	user.Repository = &project
	err = p.db.SetUserRepository(user)
	if err != nil {
		log.Error("Failed to set user repo", zap.Error(err))
		return false
	}

	log.Info("Sucessfully set user repo")
	return true
}
