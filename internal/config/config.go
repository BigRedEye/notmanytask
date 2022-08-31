package config

import (
	"time"

	"github.com/bigredeye/notmanytask/pkg/conf"
	"github.com/bigredeye/notmanytask/pkg/log"
	"github.com/pkg/errors"
)

type GitLabConfig struct {
	BaseURL string
	Group   struct {
		Name string
		ID   int
	}
	DefaultReadme string
	TaskUrlPrefix string

	Application struct {
		ClientID string
		Secret   string
	}
	Api struct {
		Token string
	}
	CIConfigPath string
}

type EndpointsConfig struct {
	HostName         string
	Home             string
	Flag             string
	Login            string
	Logout           string
	Signup           string
	Standings        string
	Retakes          string
	OauthCallback    string
	TelegramLogin    string
	TelegramCallback string

	Api struct {
		Report    string
		Flag      string
		Standings string
	}
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
	Name            string
	Secret          string
	DeadlinesURL    string
	DeadlinesFormat string
}

type GroupsConfig = []GroupConfig

type PullIntervalsConfig struct {
	Deadlines time.Duration
	Projects  *time.Duration
	Pipelines *time.Duration
}

type TelegramBotConfig struct {
	BotLogin string
	BotToken string
}

type Config struct {
	Log           log.Config
	GitLab        GitLabConfig
	Endpoints     EndpointsConfig
	Server        ServerConfig
	DataBase      DataBaseConfig
	Testing       TestingConfig
	Groups        GroupsConfig
	PullIntervals PullIntervalsConfig
	Telegram      TelegramBotConfig
}

func ParseConfig() (*Config, error) {
	config := &Config{}
	if err := conf.ParseConfig(config, conf.EnvPrefix("NMT")); err != nil {
		return nil, errors.Wrap(err, "Failed to parse config")
	}
	return config, nil
}
