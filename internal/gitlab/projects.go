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

	logger *zap.Logger
	db     *database.DataBase
	users  chan *models.User
}

func NewProjectsMaker(client *Client, db *database.DataBase) (*ProjectsMaker, error) {
	return &ProjectsMaker{client, client.logger.Named("projects"), db, make(chan *models.User, 4)}, nil
}

func (p ProjectsMaker) AsyncPrepareProject(user *models.User) {
	p.users <- user
}

func (p ProjectsMaker) Run(ctx context.Context) {
	if p.config.PullIntervals.Projects == nil {
		return
	}

	p.initializeMissingProjects()

	tick := time.Tick(*p.config.PullIntervals.Projects)
	for {
		select {
		case user := <-p.users:
			p.logger.Info("Got user from in-proc channel",
				zap.Intp("gitlab_id", user.GitlabID),
				zap.Stringp("gitlab_login", user.GitlabLogin),
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
	p.logger.Debug("Start projectsMaker iteration")
	numProjectsInitialized := 0
	defer p.logger.Debug("Finish projectsMaker iteration", zap.Int("num_projects_initialized", numProjectsInitialized))

	users, err := p.db.ListUsersWithoutRepos()
	if err != nil {
		p.logger.Error("Failed to list users without repos", zap.Error(err))
		return
	}

	for _, user := range users {
		p.logger.Info("Got user without repo from database",
			zap.Intp("gitlab_id", user.GitlabID),
			zap.Stringp("gitlab_login", user.GitlabLogin),
		)
		ok := p.maybeInitializeProject(user)
		if ok {
			numProjectsInitialized++
		}
	}
}

func (p ProjectsMaker) maybeInitializeProject(user *models.User) bool {
	log := p.logger
	if user.GitlabID == nil || user.GitlabLogin == nil {
		log.Error("Trying to initialize repo for user without login, aborting", zap.Uint("user_id", user.ID))
		return false
	}

	log = log.With(zap.Intp("gitlab_id", user.GitlabID), zap.Stringp("gitlab_login", user.GitlabLogin))

	err := p.InitializeProject(user)
	if err != nil {
		log.Error("Failed to initialize project", zap.Error(err))
		// TODO(BigRedEye): nice backoff
		time.Sleep(time.Second * 1)
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
