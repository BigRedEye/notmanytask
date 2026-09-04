package scorer

import (
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

// Deadlines policies do not own pipeline statuses: any non-successful
// pipeline must score zero regardless of the policy and submit time.
func TestScorePipelineNonSuccess(t *testing.T) {
	scorer := Scorer{}
	policy := &deadlines.ExponentialScore{Multiplier: 5 * 24 * time.Hour, Threshold: 0.3}
	dl := &deadlines.Deadlines{}
	user := &models.User{}
	task := &deadlines.Task{Task: "rewrite-in-rust", Score: 9000}
	deadline := time.Date(1969, 7, 20, 23, 17, 0, 0, time.UTC)
	group := &deadlines.TaskGroup{Deadline: deadlines.Date{Time: deadline}}

	for _, status := range []models.PipelineStatus{
		models.PipelineStatusPending,
		models.PipelineStatusRunning,
		models.PipelineStatusFailed,
	} {
		pipeline := &models.Pipeline{Status: status, StartedAt: deadline.Add(-time.Hour)}
		if score := scorer.scorePipeline(policy, dl, user, task, group, pipeline); score != 0 {
			t.Errorf("scorePipeline(status=%s) = %d, expected 0", status, score)
		}
	}
}

func TestPipelineLessPicksEarliestSuccessLatestFailure(t *testing.T) {
	early := &models.Pipeline{Status: models.PipelineStatusSuccess, StartedAt: testDeadline}
	late := &models.Pipeline{Status: models.PipelineStatusSuccess, StartedAt: testDeadline.Add(time.Hour)}
	if !pipelineLess(early, late) || pipelineLess(late, early) {
		t.Fatal("earliest success must represent the task")
	}

	earlyFail := &models.Pipeline{Status: models.PipelineStatusFailed, StartedAt: testDeadline}
	lateFail := &models.Pipeline{Status: models.PipelineStatusFailed, StartedAt: testDeadline.Add(time.Hour)}
	if !pipelineLess(lateFail, earlyFail) || pipelineLess(earlyFail, lateFail) {
		t.Fatal("latest failure must represent the task")
	}

	if !pipelineLess(late, lateFail) {
		t.Fatal("any success beats any failure")
	}
}
