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
		Report           string
		Flag             string
		Override         string
		ChangeGroup      string
		Standings        string
		ListGroupMembers string
	}
}

type ServerConfig struct {
	ListenAddress string
	CourseName    string
	HeaderName    string
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
	ShowMarks       bool
	Default         bool
}

type GroupsConfig []GroupConfig

func (g GroupsConfig) FindGroup(name string) *GroupConfig {
	for i := range g {
		if g[i].Name == name {
			return &g[i]
		}
	}
	return nil
}

func (g GroupsConfig) FindDefaultGroup() *GroupConfig {
	for i := range g {
		if g[i].Default {
			return &g[i]
		}
	}

	if len(g) == 0 {
		return nil
	}

	return &g[0]
}

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
