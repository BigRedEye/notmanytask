package web

import (
	"context"
	"net/http"

	"github.com/bigredeye/notmanytask/internal/config"
	gitlab "github.com/markbates/goth/providers/gitlab"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

var Endpoint = oauth2.Endpoint{
	AuthURL:  gitlab.AuthURL,
	TokenURL: gitlab.TokenURL,
}

type AuthClient struct {
	conf *oauth2.Config
}

func NewAuthClient(conf *config.Config) *AuthClient {
	return &AuthClient{
		conf: &oauth2.Config{
			ClientID:     conf.Platform.GitLab.Application.ClientID,
			ClientSecret: conf.Platform.GitLab.Application.Secret,
			Scopes:       []string{"read_user"},
			Endpoint:     Endpoint,
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
