package gitea

import (
	"context"
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/platform/base"

	"go.uber.org/zap"
)

type ProjectsMakerGitea struct {
	*base.ProjectsMakerBase
	*ClientGitea
}

func (p ProjectsMakerGitea) Run(ctx context.Context) {
	if p.Config.PullIntervals.Projects == nil {
		return
	}

	p.InitializeMissingProjects()

	tick := time.NewTimer(*p.Config.PullIntervals.Projects)
	for {
		select {
		case user := <-p.Users:
			p.Logger.Info("Got user from in-proc channel",
				zap.Int64p("gitea_id", user.GiteaID),
				zap.Stringp("gitea_login", user.GiteaLogin),
			)
			if !p.MaybeInitializeProject(user) {
				p.Users <- user
			}
		case <-tick.C:
			p.InitializeMissingProjects()
		case <-ctx.Done():
			p.Logger.Info("Stopping projects maker")
			return
		}
	}
}

func (p ProjectsMakerGitea) InitializeMissingProjects() {
	p.Logger.Debug("Start projectsMaker iteration")
	numProjectsInitialized := 0
	defer p.Logger.Debug("Finish projectsMaker iteration", zap.Int("num_projects_initialized", numProjectsInitialized))

	users, err := p.Db.ListUsersWithoutRepos()
	if err != nil {
		p.Logger.Error("Failed to list users without repos", zap.Error(err))
		return
	}

	for _, user := range users {
		p.Logger.Info("Got user without repo from database",
			zap.Int64p("gitea_id", user.GiteaID),
			zap.Stringp("gitea_login", user.GiteaLogin),
		)
		ok := p.MaybeInitializeProject(user)
		if ok {
			numProjectsInitialized++
		}
	}
}

func (p ProjectsMakerGitea) MaybeInitializeProject(user *models.User) bool {
	log := p.Logger
	if user.GiteaID == nil || user.GiteaLogin == nil {
		time.Sleep(time.Second * 5)
		log.Error("Trying to initialize repo for user without login, aborting", zap.Uint("user_id", user.ID))
		return false
	}

	log = log.With(zap.Int64p("gitea_id", user.GiteaID), zap.Stringp("gitea_login", user.GiteaLogin))

	err := p.InitializeProject(user)
	if err != nil {
		log.Error("Failed to initialize project", zap.Error(err))
		// TODO(BigRedEye): nice backoff
		time.Sleep(time.Second * 1)
		return false
	}

	project := p.MakeProjectURL(user)
	log = log.With(zap.String("project", project))

	user.Repository = &project
	err = p.Db.SetUserRepository(user)
	if err != nil {
		log.Error("Failed to set user repo", zap.Error(err))
		return false
	}

	log.Info("Successfully set user repo")
	return true
}
