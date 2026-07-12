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

	// TemplateProject is a project in this GitLab (path with namespace or
	// numeric id) that every new student repository is forked from. Empty
	// means an empty repository with README.
	TemplateProject string

	Application struct {
		ClientID string
		Secret   string
	}
	Api struct {
		Token string
	}
	CIConfigPath string

	// MergeRequests enables the merge-request workflow: submissions are
	// merge requests into the main branch, reviewed and auto-merged by the
	// robot. When nil, submissions are scored by pipelines only.
	MergeRequests *MergeRequestsConfig
}

type MergeRequestsConfig struct {
	// TargetBranch is the branch merge requests are opened against.
	TargetBranch string
	// ReviewTtl is how long a merge request must stay quiet (no new
	// pipelines, no new notes) before it is merged automatically. Zero keeps
	// merge requests tracked and scored but never merges them.
	ReviewTtl time.Duration
	// RobotLogin is the gitlab login of the API token owner. Merges done by
	// anyone else count as human review.
	RobotLogin string
}

func (c *MergeRequestsConfig) GetTargetBranch() string {
	if c.TargetBranch == "" {
		return "main"
	}
	return c.TargetBranch
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
	// SignupEmailPattern, when set, adds a required email field to the
	// signup form validated by this regular expression.
	SignupEmailPattern string
	Admins             []string
	Cookies            struct {
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

type SubgroupConfig struct {
	Name   string
	Secret string
}

type GroupConfig struct {
	Name            string
	Secret          string
	DeadlinesURL    string
	DeadlinesFormat string
	ShowMarks       bool
	Default         bool
	// Subgroups optionally split the group by signup secret; each subgroup
	// has its own standings view.
	Subgroups []SubgroupConfig
}

func (g *GroupConfig) FindSubgroup(name string) *SubgroupConfig {
	for i := range g.Subgroups {
		if g.Subgroups[i].Name == name {
			return &g.Subgroups[i]
		}
	}
	return nil
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

// FindBySecret resolves a signup secret to a group and, if the secret
// belongs to a subgroup, to that subgroup.
func (g GroupsConfig) FindBySecret(secret string) (group *GroupConfig, subgroup *SubgroupConfig) {
	if secret == "" {
		return nil, nil
	}
	for i := range g {
		if g[i].Secret == secret {
			return &g[i], nil
		}
		for j := range g[i].Subgroups {
			if g[i].Subgroups[j].Secret == secret {
				return &g[i], &g[i].Subgroups[j]
			}
		}
	}
	return nil, nil
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
	Deadlines     time.Duration
	Projects      *time.Duration
	Pipelines     *time.Duration
	MergeRequests *time.Duration
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
	Telegram      *TelegramBotConfig
}

func ParseConfig() (*Config, error) {
	config := &Config{}
	if err := conf.ParseConfig(config, conf.EnvPrefix("NMT")); err != nil {
		return nil, errors.Wrap(err, "Failed to parse config")
	}
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "Invalid config")
	}
	return config, nil
}

func (c *Config) Validate() error {
	if mr := c.GitLab.MergeRequests; mr != nil {
		if mr.RobotLogin == "" {
			return errors.New("gitlab.mergeRequests.robotLogin is required")
		}
		if mr.ReviewTtl < 0 {
			return errors.New("gitlab.mergeRequests.reviewTtl must not be negative")
		}
		if c.PullIntervals.MergeRequests == nil {
			return errors.New("pullIntervals.mergeRequests is required in merge request mode")
		}
	}
	return nil
}
