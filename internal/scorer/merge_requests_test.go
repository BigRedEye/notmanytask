package scorer

import (
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

type fakeProjects struct{}

func (fakeProjects) MakeProjectURL(user *models.User) string                       { return "project" }
func (fakeProjects) MakePipelineURL(*models.User, *models.Pipeline) string         { return "pipeline" }
func (fakeProjects) MakeBranchURL(*models.User, *models.Pipeline) string           { return "branch" }
func (fakeProjects) MakeMergeRequestURL(*models.User, *models.MergeRequest) string { return "mr" }
func (fakeProjects) MakeTaskURL(task string) string                                { return "task/" + task }

const (
	testRobot = "robot"
	testTask  = "intro/aplusb"
)

var (
	testDeadline = time.Date(1969, time.July, 20, 20, 17, 0, 0, time.UTC)
	testNow      = testDeadline.Add(24 * time.Hour)
)

func makeMergeRequestScorer() Scorer {
	return Scorer{
		projects: fakeProjects{},
		mergeRequests: &config.MergeRequestsConfig{
			ReviewTtl:  72 * time.Hour,
			RobotLogin: testRobot,
		},
		now: func() time.Time { return testNow },
	}
}

func makeWeekDeadlines() *deadlines.Deadlines {
	return &deadlines.Deadlines{
		Assignments: []deadlines.TaskGroup{{
			Title:    "intro",
			Deadline: deadlines.Date{Time: testDeadline},
			Tasks:    []deadlines.Task{{Task: testTask, Score: 1000}},
		}},
		Scoring: deadlines.Scoring{
			Policies: []deadlines.ScoringPolicySpec{{
				Name:   "week",
				Kind:   "linear",
				Policy: &deadlines.LinearScore{After: 7 * 24 * time.Hour, Multiplier: 0},
			}},
			Groups:       []deadlines.ScoringGroup{{Name: "default", Weight: 10, Policy: "week"}},
			DefaultGroup: "default",
		},
	}
}

func scoreMergeRequests(t *testing.T, mergeRequests []models.MergeRequest) ScoredTask {
	t.Helper()

	login := "student"
	repo := "https://gitlab/group/project"
	user := &models.User{GitlabUser: models.GitlabUser{GitlabLogin: &login, Repository: &repo}}

	d := makeWeekDeadlines()
	if err := d.BuildScoringGroups(); err != nil {
		t.Fatal(err)
	}

	scores, err := makeMergeRequestScorer().calcUserScoresImpl(
		d,
		user,
		func(string) ([]models.Pipeline, error) {
			// Pipelines never count in the merge request workflow
			return []models.Pipeline{{Task: testTask, Status: models.PipelineStatusSuccess, StartedAt: testDeadline}}, nil
		},
		func(string) ([]models.Flag, error) { return nil, nil },
		func(project string) ([]models.MergeRequest, error) {
			if project != "project" {
				t.Fatalf("unexpected project %q", project)
			}
			return mergeRequests, nil
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores.Groups) != 1 || len(scores.Groups[0].Tasks) != 1 {
		t.Fatalf("unexpected scores shape: %+v", scores)
	}
	return scores.Groups[0].Tasks[0]
}

func merged(id int, pipelineAt time.Time, mergedBy string) models.MergeRequest {
	return models.MergeRequest{
		ID: id, IID: id, Task: testTask,
		State:                 models.MergeRequestStateMerged,
		MergeUserLogin:        mergedBy,
		LastPipelineStatus:    models.PipelineStatusSuccess,
		LastPipelineCreatedAt: pipelineAt,
	}
}

func opened(id int) models.MergeRequest {
	return models.MergeRequest{
		ID: id, IID: id, Task: testTask,
		State:                 models.MergeRequestStateOpened,
		MergeStatus:           "can_be_merged",
		LastPipelineStatus:    models.PipelineStatusSuccess,
		LastPipelineCreatedAt: testDeadline,
	}
}

func TestMergeRequestNoEvidence(t *testing.T) {
	task := scoreMergeRequests(t, nil)
	if task.Status != TaskStatusAssigned || task.Score != 0 || task.PipelineUrl != "" {
		t.Fatalf("pipelines must not count: %+v", task)
	}
}

func TestMergeRequestMerged(t *testing.T) {
	cases := []struct {
		name   string
		mr     models.MergeRequest
		score  int
		review bool
	}{
		{"in time by robot", merged(1, testDeadline.Add(-time.Hour), testRobot), 1000, false},
		{"in time by human", merged(1, testDeadline.Add(-time.Hour), "teacher"), 1000, true},
		{"half week late", merged(1, testDeadline.Add(3*24*time.Hour+12*time.Hour), testRobot), 500, false},
		{"week late", merged(1, testDeadline.Add(8*24*time.Hour), testRobot), 0, false},
	}
	for _, c := range cases {
		task := scoreMergeRequests(t, []models.MergeRequest{c.mr})
		if task.Status != TaskStatusSuccess || task.Score != c.score || task.HasReview != c.review || task.PipelineUrl != "mr" {
			t.Errorf("%s: %+v", c.name, task)
		}
	}

	// Merged with a non-successful pipeline (a re-check failed) shows as
	// failed and gives no score
	mr := merged(1, testDeadline, testRobot)
	mr.LastPipelineStatus = models.PipelineStatusFailed
	if task := scoreMergeRequests(t, []models.MergeRequest{mr}); task.Score != 0 || task.Status != TaskStatusFailed || task.Message != "pipeline failed" {
		t.Fatalf("merged failed pipeline: %+v", task)
	}

	// Merged with a success but no pipeline timestamp gives no score either:
	// zero time is before any deadline and would mean full credit
	mr = merged(1, time.Time{}, testRobot)
	if task := scoreMergeRequests(t, []models.MergeRequest{mr}); task.Score != 0 || task.Status != TaskStatusFailed {
		t.Fatalf("merged without pipeline timestamp: %+v", task)
	}
}

func TestTimeToMerge(t *testing.T) {
	s := makeMergeRequestScorer()
	mr := opened(1) // pipeline at testDeadline, now = testDeadline + 24h, ttl 72h
	if got := s.timeToMerge(&mr); got != "48h0m0s" {
		t.Fatalf("time left: %q", got)
	}
	mr.LastPipelineCreatedAt = testNow.Add(-100 * time.Hour)
	if got := s.timeToMerge(&mr); got != "" {
		t.Fatalf("overdue must show nothing, got %q", got)
	}
	s.mergeRequests.ReviewTtl = 0
	fresh := opened(1)
	if got := s.timeToMerge(&fresh); got != "" {
		t.Fatalf("no auto-merge must show nothing, got %q", got)
	}
}

func TestMergeRequestOpen(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*models.MergeRequest)
		status  TaskStatus
		message string
	}{
		{"pending", func(*models.MergeRequest) {}, TaskStatusReviewPending, "48h0m0s"},
		{"pending after note", func(m *models.MergeRequest) {
			m.LastNoteCreatedAt = testDeadline.Add(12 * time.Hour)
		}, TaskStatusReviewPending, "60h0m0s"},
		{"pipeline failed", func(m *models.MergeRequest) {
			m.LastPipelineStatus = models.PipelineStatusFailed
		}, TaskStatusFailed, "pipeline failed"},
		{"conflict", func(m *models.MergeRequest) {
			m.MergeStatus = models.MergeRequestStatusCannotBeMerged
		}, TaskStatusFailed, "merge conflict"},
		{"extra changes", func(m *models.MergeRequest) {
			m.ExtraChanges = true
		}, TaskStatusFailed, "extra changes"},
		{"no changes", func(m *models.MergeRequest) {
			m.NoChanges = true
		}, TaskStatusFailed, "no changes"},
		{"on review", func(m *models.MergeRequest) {
			m.UserNotesCount = 1
			m.HasUnresolvedNotes = true
		}, TaskStatusReviewUnresolved, ""},
		{"review resolved", func(m *models.MergeRequest) {
			m.UserNotesCount = 1
		}, TaskStatusReviewResolved, "48h0m0s"},
		{"closed", func(m *models.MergeRequest) {
			m.State = models.MergeRequestStateClosed
		}, TaskStatusReviewPending, ""},
	}
	for _, c := range cases {
		mr := opened(1)
		c.mutate(&mr)
		task := scoreMergeRequests(t, []models.MergeRequest{mr})
		if task.Status != c.status || task.Message != c.message || task.Score != 0 {
			t.Errorf("%s: %+v", c.name, task)
		}
		if task.HasReview != (mr.UserNotesCount > 0) {
			t.Errorf("%s: HasReview=%v", c.name, task.HasReview)
		}
	}
}

func TestMergeRequestSelection(t *testing.T) {
	// A merged request is never shadowed by a later failed or open one
	failed := opened(2)
	failed.LastPipelineStatus = models.PipelineStatusFailed
	task := scoreMergeRequests(t, []models.MergeRequest{failed, merged(1, testDeadline, testRobot), opened(3)})
	if task.Status != TaskStatusSuccess || task.Score != 1000 {
		t.Fatalf("merged must win: %+v", task)
	}

	// Among open ones the most actionable state wins: failed pipeline over pending
	task = scoreMergeRequests(t, []models.MergeRequest{opened(1), failed})
	if task.Status != TaskStatusFailed {
		t.Fatalf("failed must win over pending: %+v", task)
	}

	// Among merged requests the earliest pipeline wins, regardless of row order
	late, early := merged(1, testDeadline.Add(8*24*time.Hour), testRobot), merged(2, testDeadline, testRobot)
	for _, rows := range [][]models.MergeRequest{{late, early}, {early, late}} {
		if task = scoreMergeRequests(t, rows); task.Score != 1000 {
			t.Fatalf("earliest merged pipeline must win: %+v", task)
		}
	}

	// A merged request with a failed pipeline (merged by hand) does not shadow
	// a later merged one with a green pipeline
	handMerged := merged(1, testDeadline, "teacher")
	handMerged.LastPipelineStatus = models.PipelineStatusFailed
	if task = scoreMergeRequests(t, []models.MergeRequest{handMerged, merged(2, testDeadline, testRobot)}); task.Score != 1000 {
		t.Fatalf("creditable merged request must win: %+v", task)
	}
	if task = scoreMergeRequests(t, []models.MergeRequest{handMerged}); task.Status != TaskStatusFailed || task.Score != 0 {
		t.Fatalf("hand-merged failed pipeline: %+v", task)
	}

	// Notes on any request of the task count as review
	noted := opened(1)
	noted.UserNotesCount = 2
	noted.State = models.MergeRequestStateClosed
	task = scoreMergeRequests(t, []models.MergeRequest{noted, merged(2, testDeadline, testRobot)})
	if !task.HasReview {
		t.Fatalf("review on closed request must be visible: %+v", task)
	}
}

func TestMergeRequestOverride(t *testing.T) {
	login := "student"
	repo := "https://gitlab/group/project"
	user := &models.User{GitlabUser: models.GitlabUser{GitlabLogin: &login, Repository: &repo}}
	d := makeWeekDeadlines()
	if err := d.BuildScoringGroups(); err != nil {
		t.Fatal(err)
	}

	scores, err := makeMergeRequestScorer().calcUserScoresImpl(
		d, user,
		func(string) ([]models.Pipeline, error) { return nil, nil },
		func(string) ([]models.Flag, error) { return nil, nil },
		func(string) ([]models.MergeRequest, error) { return []models.MergeRequest{opened(1)}, nil },
		[]models.OverriddenScore{{GitlabLogin: login, Task: testTask, Score: 42, Status: models.PipelineStatusSuccess}},
	)
	if err != nil {
		t.Fatal(err)
	}
	task := scores.Groups[0].Tasks[0]
	if !task.Overridden || task.Score != 42 || task.Status != TaskStatusSuccess {
		t.Fatalf("override must win: %+v", task)
	}
}
