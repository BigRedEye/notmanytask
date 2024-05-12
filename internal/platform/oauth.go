package platform

import (
	"net/http"

	gitea "code.gitea.io/sdk/gitea"
	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
)

type User struct {
	ID    int
	Login string
}

type UserGitea struct {
	ID    int64
	Login string
}

func GetOAuthGitLabUser(token string) (*User, error) {
	client, err := gitlab.NewOAuthClient(token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create gitlab client")
	}

	user, resp, err := client.Users.CurrentUser()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current user")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to get current user: %s", resp.Status)
	}

	return &User{
		ID:    user.ID,
		Login: user.Username,
	}, nil
}

func GetOAuthGiteaUser(conf *config.Config, token string) (*UserGitea, error) {
	client, err := gitea.NewClient(conf.Platform.Gitea.BaseURL, gitea.SetToken(token))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create gitea client")
	}

	user, _, err := client.GetMyUserInfo()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current user")
	}

	return &UserGitea{
		ID:    user.ID,
		Login: user.UserName,
	}, nil
}
