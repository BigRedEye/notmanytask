package web

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/gitlab"
)

type AuthClient struct {
	conf *oauth2.Config
}

func NewAuthClient(config *Config) *AuthClient {
	return &AuthClient{
		conf: &oauth2.Config{
			ClientID:     config.GitLab.ClientId,
			ClientSecret: config.GitLab.Secret,
			Scopes:       []string{"read_user"},
			Endpoint:     gitlab.Endpoint,
			RedirectURL:  config.Endpoints.HostName + config.Endpoints.OauthCallback,
		},
	}
}

func (c *AuthClient) LoginUrl(state string) string {
	return c.conf.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (c *AuthClient) Exchange(ctx context.Context, code string) (token *oauth2.Token, err error) {
	token, err = c.conf.Exchange(ctx, code)
	err = errors.Wrap(err, "Failed to get oauth2 token pair from GitLab")
	return
}

func (c *AuthClient) Client(ctx context.Context, token *oauth2.Token) *http.Client {
	return c.conf.Client(ctx, token)
}
