package config

import (
	"github.com/bigredeye/notmanytask/pkg/conf"
	"github.com/pkg/errors"
)

type GitLabConfig struct {
	BaseURL string
	Group   struct {
		Name string
		ID   int
	}
	DefaultReadme string

	Application struct {
		ClientID string
		Secret   string
	}
	Api struct {
		Token string
	}
}

type EndpointsConfig struct {
	HostName      string
	Home          string
	Login         string
	Logout        string
	Signup        string
	OauthCallback string
}
type ServerConfig struct {
	ListenAddress string
	Cookies       struct {
		AuthenticationKey string
		EncryptionKey     string
	}
}

type DataBaseConfig struct {
	Host string
	Port uint16
	User string
	Pass string
	Name string
}

type TestingConfig struct {
	Tokens []string
}

type GroupConfig struct {
	Name         string
	Secret       string
	DeadlinesURL string
}

type GroupsConfig = []GroupConfig

type Config struct {
	GitLab    GitLabConfig
	Endpoints EndpointsConfig
	Server    ServerConfig
	DataBase  DataBaseConfig
	Testing   TestingConfig
	Groups    GroupsConfig
}

func ParseConfig() (*Config, error) {
	config := &Config{}
	if err := conf.ParseConfig(config, conf.EnvPrefix("NMT")); err != nil {
		return nil, errors.Wrap(err, "Failed to parse config")
	}
	return config, nil
}
