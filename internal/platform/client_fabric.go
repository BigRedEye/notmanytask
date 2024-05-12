package platform

import (
	"fmt"

	gitea "code.gitea.io/sdk/gitea"
	"github.com/alexsergivan/transliterator"
	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/platform/base"
	gitea_client "github.com/bigredeye/notmanytask/internal/platform/gitea"
	gitlab_client "github.com/bigredeye/notmanytask/internal/platform/gitlab"
	"github.com/pkg/errors"
	gitlab "github.com/xanzy/go-gitlab"

	"go.uber.org/zap"
)

func NewClient(conf *config.Config, logger *zap.Logger) (base.ClientInterface, error) {
	switch conf.Platform.Mode {
	case "gitlab":
		client, err := gitlab.NewClient(conf.Platform.GitLab.Api.Token, gitlab.WithBaseURL(conf.Platform.GitLab.BaseURL))
		if err != nil {
			return nil, errors.Wrap(err, "Failed to create gitlab client")
		}
		return &gitlab_client.ClientGitlab{
			ClientBase: &base.ClientBase{
				Config:   conf,
				Logger:   logger,
				Translit: transliterator.NewTransliterator(nil),
			},
			Gitlab: client,
		}, nil
	case "gitea":
		client, err := gitea.NewClient(conf.Platform.GitLab.BaseURL, gitea.SetToken(conf.Platform.Gitea.Api.Token))
		if err != nil {
			return nil, errors.Wrap(err, "Failed to create gitea client")
		}
		return &gitea_client.ClientGitea{
			ClientBase: &base.ClientBase{
				Config:   conf,
				Logger:   logger,
				Translit: transliterator.NewTransliterator(nil),
			},
			Gitea: client,
		}, nil
	default:
		return nil, errors.Wrap(errors.Errorf("Unknown platform mode: %s", conf.Platform.Mode), fmt.Sprintf("Failed to create client for platform %s", conf.Platform.Mode))
	}
}
