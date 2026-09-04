package web

import (
	"testing"

	"github.com/bigredeye/notmanytask/internal/models"
)

func TestParseBenchmarkMetric(t *testing.T) {
	for _, raw := range []string{"NaN", "+Inf", "-Inf", "not-a-number"} {
		if _, err := parseBenchmarkMetric(raw); err == nil {
			t.Errorf("parseBenchmarkMetric(%q) unexpectedly succeeded", raw)
		}
	}

	metric, err := parseBenchmarkMetric("12.5")
	if err != nil {
		t.Fatal(err)
	}
	if metric != 12.5 {
		t.Fatalf("unexpected metric: got %v, want 12.5", metric)
	}
}

func TestValidateBenchmarkReportIdentity(t *testing.T) {
	user := &models.User{GitlabUser: models.GitlabUser{ProjectName: "alice-project"}}
	pipeline := &models.Pipeline{ID: 42, Project: "alice-project", Task: "bench", Status: models.PipelineStatusSuccess}
	if err := validateBenchmarkReport(user, pipeline, "bench", "alice-project"); err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}

	tests := []struct {
		name     string
		user     *models.User
		pipeline *models.Pipeline
		task     string
		project  string
	}{
		{"user project", &models.User{GitlabUser: models.GitlabUser{ProjectName: "bob-project"}}, pipeline, "bench", "alice-project"},
		{"pipeline project", user, &models.Pipeline{ID: 42, Project: "bob-project", Task: "bench", Status: models.PipelineStatusSuccess}, "bench", "alice-project"},
		{"pipeline task", user, &models.Pipeline{ID: 42, Project: "alice-project", Task: "other", Status: models.PipelineStatusSuccess}, "bench", "alice-project"},
		{"pipeline status", user, &models.Pipeline{ID: 42, Project: "alice-project", Task: "bench", Status: models.PipelineStatusFailed}, "bench", "alice-project"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateBenchmarkReport(test.user, test.pipeline, test.task, test.project); err == nil {
				t.Fatal("mismatched report was accepted")
			}
		})
	}
}
