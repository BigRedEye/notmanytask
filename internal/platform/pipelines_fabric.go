package platform

import (
	"fmt"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/platform/base"
	gitea_client "github.com/bigredeye/notmanytask/internal/platform/gitea"
	gitlab_client "github.com/bigredeye/notmanytask/internal/platform/gitlab"

	"github.com/pkg/errors"
)

func NewPipelinesFetcher(conf *config.Config, client base.ClientInterface, db *database.DataBase) (base.PipelinesFetcherInterface, error) {
	switch conf.Platform.Mode {
	case "gitlab":
		client_gitlab, done := client.(*gitlab_client.ClientGitlab)
		if !done {
			return nil, errors.Wrap(nil, "failed to cast client to gitlab")
		}

		return &gitlab_client.PipelinesFetcherGitlab{
			ClientGitlab: client_gitlab,
			PipelinesFetcherBase: &base.PipelinesFetcherBase{
				Logger: client_gitlab.Logger.Named("pipelines"),
				Db:     db,
			},
		}, nil
	case "gitea":
		client_gitea, done := client.(*gitea_client.ClientGitea)
		if !done {
			return nil, errors.Wrap(nil, "failed to cast client to gitlab")
		}

		return &gitea_client.PipelinesFetcherGitea{
			ClientGitea: client_gitea,
			PipelinesFetcherBase: &base.PipelinesFetcherBase{
				Logger: client_gitea.Logger.Named("pipelines"),
				Db:     db,
			},
		}, nil
	default:
		return nil, errors.Wrap(errors.Errorf("Unknown platform mode: %s", conf.Platform.Mode), fmt.Sprintf("Failed to create pipeline fetcher for platform %s", conf.Platform.Mode))
	}

}
