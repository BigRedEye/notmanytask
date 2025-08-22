package gitlab

import (
	"net/http"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
)

type User struct {
	ID    int
	Login string
}

func GetOAuthGitLabUser(token, baseURL string) (*User, error) {
	client, err := gitlab.NewOAuthClient(token, gitlab.WithBaseURL(baseURL))
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
