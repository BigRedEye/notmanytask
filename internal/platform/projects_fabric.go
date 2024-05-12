package platform

import (
	"fmt"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/platform/base"
	gitea_client "github.com/bigredeye/notmanytask/internal/platform/gitea"
	gitlab_client "github.com/bigredeye/notmanytask/internal/platform/gitlab"
	"github.com/pkg/errors"
)

func NewProjectsMaker(conf *config.Config, client base.ClientInterface, db *database.DataBaseProxy) (base.ProjectsMakerInterface, error) {
	switch conf.Platform.Mode {
	case config.GitlabMode:
		client_gitlab, done := client.(*gitlab_client.ClientGitlab)
		if !done {
			return nil, errors.Wrap(nil, "Projects maker: client is not gitlab")
		}

		return &gitlab_client.ProjectsMakerGitlab{
			ClientGitlab: client_gitlab,
			ProjectsMakerBase: &base.ProjectsMakerBase{
				Logger: client_gitlab.Logger.Named("projects"),
				Db:     db,
				Users:  make(chan *models.User, 4),
			},
		}, nil
	case config.GiteaMode:
		client_gitea, done := client.(*gitea_client.ClientGitea)
		if !done {
			return nil, errors.Wrap(nil, "Projects maker: client is not gitea")
		}

		return &gitea_client.ProjectsMakerGitea{
			ClientGitea: client_gitea,
			ProjectsMakerBase: &base.ProjectsMakerBase{
				Logger: client_gitea.Logger.Named("projects"),
				Db:     db,
				Users:  make(chan *models.User, 4),
			},
		}, nil
	default:
		return nil, errors.Wrap(errors.Errorf("Unknown platform mode: %s", conf.Platform.Mode), fmt.Sprintf("Failed to create projects maker for platform %s", conf.Platform.Mode))
	}

}
