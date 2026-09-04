package scorer

import (
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
)

func TestLoadUserPipelinesFallsBackAfterBan(t *testing.T) {
	repository := "https://gitlab.example/course/alice"
	user := &models.User{GitlabUser: models.GitlabUser{Repository: &repository}}
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	provider := func(project string) ([]models.Pipeline, error) {
		return []models.Pipeline{
			{ID: 1, Project: project, Task: "task", Status: models.PipelineStatusSuccess, StartedAt: older},
			{ID: 2, Project: project, Task: "task", Status: models.PipelineStatusSuccess, StartedAt: newer},
		}, nil
	}

	pipelines, err := (Scorer{}).loadUserPipelines(user, provider, submissionBans{1: {PipelineID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if got := pipelines["task"]; got == nil || got.ID != 2 {
		t.Fatalf("expected unbanned fallback pipeline 2, got %#v", got)
	}
}
