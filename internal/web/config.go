package web

import (
	"github.com/bigredeye/notmanytask/pkg/conf"
	"github.com/pkg/errors"
)

type Config struct {
	GitLab struct {
		ClientId string
		Secret   string
	}
	Endpoints struct {
		Redirect string
	}
	Server struct {
		ListenAddress string
	}
}

func ParseConfig() (*Config, error) {
	config := &Config{}
	if err := conf.ParseConfig(config, conf.EnvPrefix("WEB")); err != nil {
		return nil, errors.Wrap(err, "Failed to parse config")
	}
	return config, nil
}
