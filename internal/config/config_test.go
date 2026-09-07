package config

import (
	"testing"
	"time"
)

func TestFindBySecret(t *testing.T) {
	groups := GroupsConfig{
		{Name: "hse", Secret: "ihatecpp"},
		{Name: "ami", Subgroups: []SubgroupConfig{
			{Name: "254-1", Secret: "s1"},
			{Name: "254-2", Secret: "s2"},
		}},
	}

	group, subgroup := groups.FindBySecret("ihatecpp")
	if group == nil || group.Name != "hse" || subgroup != nil {
		t.Fatalf("plain group secret: got %v %v", group, subgroup)
	}

	group, subgroup = groups.FindBySecret("s2")
	if group == nil || group.Name != "ami" || subgroup == nil || subgroup.Name != "254-2" {
		t.Fatalf("subgroup secret: got %v %v", group, subgroup)
	}

	for _, secret := range []string{"", "unknown"} {
		if group, subgroup := groups.FindBySecret(secret); group != nil || subgroup != nil {
			t.Fatalf("secret %q must not match", secret)
		}
	}
}

func TestValidateMergeRequests(t *testing.T) {
	interval := 60 * time.Second
	cases := []struct {
		name   string
		config Config
		valid  bool
	}{
		{"pipeline mode", Config{}, true},
		{"complete", Config{
			GitLab:        GitLabConfig{MergeRequests: &MergeRequestsConfig{ReviewTtl: 1, RobotLogin: "robot"}},
			PullIntervals: PullIntervalsConfig{MergeRequests: &interval},
		}, true},
		{"no robot", Config{
			GitLab:        GitLabConfig{MergeRequests: &MergeRequestsConfig{ReviewTtl: 1}},
			PullIntervals: PullIntervalsConfig{MergeRequests: &interval},
		}, false},
		{"no auto-merge", Config{
			GitLab:        GitLabConfig{MergeRequests: &MergeRequestsConfig{RobotLogin: "robot"}},
			PullIntervals: PullIntervalsConfig{MergeRequests: &interval},
		}, true},
		{"negative ttl", Config{
			GitLab:        GitLabConfig{MergeRequests: &MergeRequestsConfig{ReviewTtl: -1, RobotLogin: "robot"}},
			PullIntervals: PullIntervalsConfig{MergeRequests: &interval},
		}, false},
		{"no interval", Config{
			GitLab: GitLabConfig{MergeRequests: &MergeRequestsConfig{ReviewTtl: 1, RobotLogin: "robot"}},
		}, false},
	}
	if err := (&Config{}).Validate(); err == nil {
		t.Fatal("gitlab.group.name must be required")
	}
	for _, c := range cases {
		c.config.GitLab.Group.Name = "g"
		err := c.config.Validate()
		if (err == nil) != c.valid {
			t.Errorf("%s: valid=%v, err=%v", c.name, c.valid, err)
		}
	}

	if (&MergeRequestsConfig{}).GetTargetBranch() != "main" {
		t.Fatal("default target branch must be main")
	}
}

func TestValidateTelegramProxy(t *testing.T) {
	base := Config{}
	base.GitLab.Group.Name = "g"
	for proxy, valid := range map[string]bool{
		"":                              true,
		"socks5://127.0.0.1:1080":       true,
		"socks5://user:pass@proxy:1080": true,
		"http://proxy:3128":             true,
		"proxy:1080":                    false,
		"ftp://proxy":                   false,
	} {
		c := base
		c.Telegram = &TelegramBotConfig{Proxy: proxy}
		if err := c.Validate(); (err == nil) != valid {
			t.Errorf("proxy %q: valid=%v, err=%v", proxy, valid, err)
		}
	}
}
