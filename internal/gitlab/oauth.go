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

func GetOAuthGitLabUser(token string) (*User, error) {
	client, err := gitlab.NewOAuthClient(token)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create gitlab client")
	}

	user, resp, err := client.Users.CurrentUser()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get current user")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("Failed to get current user: %s", resp.Status)
	}

	return &User{
		ID:    user.ID,
		Login: user.Username,
	}, nil
}
