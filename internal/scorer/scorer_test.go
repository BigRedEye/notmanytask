package scorer

import (
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
	"gopkg.in/yaml.v2"
)

const someStrangeDeadlines = `
- group:    42-cpp-sucks
  start:    17-07-1968 18:00
  deadline: 20-07-1969 23:17
  tasks:
    - task: fly-me-to-the-moon
      score: 100
    - task: rewrite-in-rust
      score: 9000
    - task: rewrite-in-go
      score: 50
    - task: rewrite-in-agc-assembly
      score: 300
`

func mustParse(t time.Time, err error) time.Time {
	if err != nil {
		panic(err)
	}
	return t
}

func makePipeline(timePoint string, status models.PipelineStatus) *models.Pipeline {
	return &models.Pipeline{
		Status:    status,
		StartedAt: mustParse(time.Parse("02-01-2006 15:04", timePoint)),
	}
}

func checkScore(t *testing.T, groups deadlines.Deadlines, submitDate string, expectedScore int, scorer scoringFunc) {
	score := scorer(&groups[0].Tasks[1], &groups[0], makePipeline(submitDate, models.PipelineStatusSuccess))
	if score != expectedScore {
		t.Fatalf("Invalid score: %d, expected: %d", score, expectedScore)
	}
}

func checkFailedScore(t *testing.T, groups deadlines.Deadlines, submitDate string, scorer scoringFunc, status models.PipelineStatus) {
	score := scorer(&groups[0].Tasks[1], &groups[0], makePipeline(submitDate, status))
	if score != 0 {
		t.Fatalf("Invalid score: %d, expected: %d", score, 0)
	}
}

func TestLinearScoring(t *testing.T) {
	groups := deadlines.Deadlines{}
	err := yaml.Unmarshal([]byte(someStrangeDeadlines), &groups)
	if err != nil {
		panic(err)
	}

	checkScore(t, groups, "19-07-1969 23:00", 9000, linearScore)
	checkScore(t, groups, "19-07-1979 23:00", 4500, linearScore)
	checkScore(t, groups, "20-07-1969 20:18", 8999, linearScore)
	checkScore(t, groups, "20-07-1969 21:17", 8973, linearScore)
	checkScore(t, groups, "21-07-1969 13:17", 8544, linearScore)
	checkScore(t, groups, "22-07-1969 13:17", 7901, linearScore)
	checkScore(t, groups, "27-07-1969 20:14", 4501, linearScore)
	checkFailedScore(t, groups, "19-07-1969 23:00", linearScore, models.PipelineStatusPending)
	checkFailedScore(t, groups, "19-07-1969 23:00", linearScore, models.PipelineStatusRunning)
	checkFailedScore(t, groups, "19-07-1969 23:00", linearScore, models.PipelineStatusFailed)
}

func TestExponentialScoring(t *testing.T) {
	groups := deadlines.Deadlines{}
	err := yaml.Unmarshal([]byte(someStrangeDeadlines), &groups)
	if err != nil {
		panic(err)
	}

	checkScore(t, groups, "19-07-1969 23:00", 9000, exponentialScore) // before deadline
	checkScore(t, groups, "19-07-1979 23:00", 2700, exponentialScore) // ten years after deadline
	checkScore(t, groups, "20-07-1969 20:17", 9000, exponentialScore) // just at deadline
	checkScore(t, groups, "20-07-1969 20:18", 8998, exponentialScore) // one minute after deadline
	checkScore(t, groups, "20-07-1969 21:17", 8925, exponentialScore) // one hour after deadline
	checkScore(t, groups, "21-07-1969 13:17", 7811, exponentialScore) // next day after deadline
	checkScore(t, groups, "22-07-1969 13:17", 6395, exponentialScore) // two days after deadline
	checkScore(t, groups, "27-07-1969 20:14", 2700, exponentialScore) // one week after deadline
	checkFailedScore(t, groups, "19-07-1969 23:00", exponentialScore, models.PipelineStatusPending)
	checkFailedScore(t, groups, "19-07-1969 23:00", exponentialScore, models.PipelineStatusRunning)
	checkFailedScore(t, groups, "19-07-1969 23:00", exponentialScore, models.PipelineStatusFailed)
}
