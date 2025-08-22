package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"github.com/bigredeye/notmanytask/internal/config"
)

type AuthClient struct {
	conf *oauth2.Config
}

func makeGitlabOauthEndpoint(baseurl string) oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  fmt.Sprintf("%s/oauth/authorize", baseurl),
		TokenURL: fmt.Sprintf("%s/oauth/token", baseurl),
	}
}

func NewAuthClient(conf *config.Config) *AuthClient {
	return &AuthClient{
		conf: &oauth2.Config{
			ClientID:     conf.GitLab.Application.ClientID,
			ClientSecret: conf.GitLab.Application.Secret,
			Scopes:       []string{"read_user"},
			Endpoint:     makeGitlabOauthEndpoint(conf.GitLab.BaseURL),
			RedirectURL:  conf.Endpoints.HostName + conf.Endpoints.OauthCallback,
		},
	}
}

func (c *AuthClient) LoginURL(state string) string {
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
