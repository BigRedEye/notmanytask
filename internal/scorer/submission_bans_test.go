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

func TestLoadUserMergeRequestsSkipsBannedPipeline(t *testing.T) {
	repository := "https://gitlab.example/course/alice"
	user := &models.User{GitlabUser: models.GitlabUser{Repository: &repository}}
	provider := func(project string) ([]models.MergeRequest, error) {
		return []models.MergeRequest{
			{ID: 1, Project: project, Task: "task", State: models.MergeRequestStateMerged, LastPipelineID: 10, LastPipelineStatus: models.PipelineStatusSuccess, LastPipelineCreatedAt: time.Now()},
			{ID: 2, Project: project, Task: "task", State: models.MergeRequestStateOpened, LastPipelineID: 11, LastPipelineStatus: models.PipelineStatusSuccess, LastPipelineCreatedAt: time.Now()},
		}, nil
	}

	mergeRequests, err := (Scorer{}).loadUserMergeRequests(user, provider, submissionBans{10: {PipelineID: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if got := mergeRequests["task"]; got == nil || got.MergeRequest.ID != 2 {
		t.Fatalf("expected the open request 2 once the merged one is banned, got %#v", got)
	}
}
