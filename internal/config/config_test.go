package config

import (
	"testing"
	"time"
)

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
	for _, c := range cases {
		err := c.config.Validate()
		if (err == nil) != c.valid {
			t.Errorf("%s: valid=%v, err=%v", c.name, c.valid, err)
		}
	}

	if (&MergeRequestsConfig{}).GetTargetBranch() != "main" {
		t.Fatal("default target branch must be main")
	}
}
