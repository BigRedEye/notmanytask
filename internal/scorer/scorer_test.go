package scorer

import (
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

const testTaskScore = 9000

var testDeadline = time.Date(1969, time.July, 20, 20, 17, 0, 0, time.UTC)

func makePipeline(startedAt time.Time, status models.PipelineStatus) *models.Pipeline {
	return &models.Pipeline{
		Status:    status,
		StartedAt: startedAt,
	}
}

func checkScore(t *testing.T, policy deadlines.ScoringPolicy, submitTime time.Time, expectedScore int) {
	t.Helper()

	task := deadlines.Task{Score: testTaskScore}
	group := deadlines.TaskGroup{Deadline: deadlines.Date{Time: testDeadline}}
	score := (Scorer{}).scorePipeline(
		policy,
		&deadlines.Deadlines{},
		&models.User{},
		&task,
		&group,
		makePipeline(submitTime, models.PipelineStatusSuccess),
	)
	if score != expectedScore {
		t.Fatalf("invalid score: %d, expected: %d", score, expectedScore)
	}
}

func checkFailedScore(t *testing.T, policy deadlines.ScoringPolicy, status models.PipelineStatus) {
	t.Helper()

	task := deadlines.Task{Score: testTaskScore}
	group := deadlines.TaskGroup{Deadline: deadlines.Date{Time: testDeadline}}
	score := (Scorer{}).scorePipeline(
		policy,
		&deadlines.Deadlines{},
		&models.User{},
		&task,
		&group,
		makePipeline(testDeadline.Add(-time.Minute), status),
	)
	if score != 0 {
		t.Fatalf("invalid score: %d, expected: %d", score, 0)
	}
}

func TestLinearScoring(t *testing.T) {
	policy := &deadlines.LinearScore{
		After:      7 * 24 * time.Hour,
		Multiplier: 0.5,
	}

	checkScore(t, policy, testDeadline.Add(-time.Minute), testTaskScore)
	checkScore(t, policy, testDeadline, testTaskScore)
	checkScore(t, policy, testDeadline.Add(time.Minute), 8999)
	checkScore(t, policy, testDeadline.Add(time.Hour), 8973)
	checkScore(t, policy, testDeadline.Add(17*time.Hour), 8544)
	checkScore(t, policy, testDeadline.Add(41*time.Hour), 7901)
	checkScore(t, policy, testDeadline.Add(policy.After-3*time.Minute), 4501)
	checkScore(t, policy, testDeadline.Add(policy.After), 4500)
	checkFailedScore(t, policy, models.PipelineStatusPending)
	checkFailedScore(t, policy, models.PipelineStatusRunning)
	checkFailedScore(t, policy, models.PipelineStatusFailed)
}

func TestExponentialScoring(t *testing.T) {
	policy := &deadlines.ExponentialScore{
		Multiplier: 5 * 24 * time.Hour,
		Threshold:  0.3,
	}

	checkScore(t, policy, testDeadline.Add(-time.Minute), testTaskScore)
	checkScore(t, policy, testDeadline, testTaskScore)
	checkScore(t, policy, testDeadline.Add(time.Minute), 8998)
	checkScore(t, policy, testDeadline.Add(time.Hour), 8925)
	checkScore(t, policy, testDeadline.Add(17*time.Hour), 7811)
	checkScore(t, policy, testDeadline.Add(41*time.Hour), 6395)
	checkScore(t, policy, testDeadline.Add(7*24*time.Hour-3*time.Minute), 2700)
	checkFailedScore(t, policy, models.PipelineStatusPending)
	checkFailedScore(t, policy, models.PipelineStatusRunning)
	checkFailedScore(t, policy, models.PipelineStatusFailed)
}
