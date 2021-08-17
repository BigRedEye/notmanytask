package web

import (
	"github.com/bigredeye/notmanytask/pkg/conf"
	"github.com/pkg/errors"
)

type Config struct {
	GitLab struct {
		ClientID string
		Secret   string
	}
	Endpoints struct {
		HostName      string
		Home          string
		Login         string
		Signup        string
		OauthCallback string
	}
	Server struct {
		ListenAddress string
		Cookies       struct {
			AuthenticationKey string
			EncryptionKey     string
		}
	}
}

func ParseConfig() (*Config, error) {
	config := &Config{}
	if err := conf.ParseConfig(config, conf.EnvPrefix("WEB")); err != nil {
		return nil, errors.Wrap(err, "Failed to parse config")
	}
	return config, nil
}
