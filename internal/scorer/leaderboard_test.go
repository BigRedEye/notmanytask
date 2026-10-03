package scorer

import (
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

func expectEqual(t *testing.T, got, want int, format string, args ...interface{}) {
	t.Helper()
	if got != want {
		t.Errorf(format+": got %d, want %d", append(args, got, want)...)
	}
}

func TestLeaderboardScore(t *testing.T) {
	// First place gets base*(1+bonus), last place gets exactly the base score.
	expectEqual(t, leaderboardScore(100, 1.0, 1, 10), 200, "first place")
	expectEqual(t, leaderboardScore(100, 1.0, 10, 10), 100, "last place")

	// Monotonic in rank.
	prev := leaderboardScore(100, 1.0, 1, 10)
	for rank := 2; rank <= 10; rank++ {
		cur := leaderboardScore(100, 1.0, rank, 10)
		if cur > prev {
			t.Errorf("rank %d score %d is greater than rank %d score %d", rank, cur, rank-1, prev)
		}
		prev = cur
	}

	// Single participant is the first place.
	expectEqual(t, leaderboardScore(100, 0.5, 1, 1), 150, "single participant")

	// Unknown rank keeps the base score.
	expectEqual(t, leaderboardScore(100, 1.0, 0, 10), 100, "zero rank")
	expectEqual(t, leaderboardScore(100, 1.0, 3, 0), 100, "empty board")

	// Zero bonus disables the leaderboard influence entirely.
	expectEqual(t, leaderboardScore(100, 0.0, 1, 10), 100, "zero bonus")
}

func TestLeaderboardEntryOrder(t *testing.T) {
	now := time.Now()
	fast := &LeaderboardEntry{Metric: 10.0, SubmittedAt: now}
	slow := &LeaderboardEntry{Metric: 20.0, SubmittedAt: now.Add(-time.Hour)}
	if !entryLess(fast, slow) || entryLess(slow, fast) {
		t.Error("smaller metric must rank higher")
	}

	// Ties are broken by submission time: earlier wins.
	early := &LeaderboardEntry{Metric: 10.0, SubmittedAt: now.Add(-time.Hour)}
	late := &LeaderboardEntry{Metric: 10.0, SubmittedAt: now}
	if !entryLess(early, late) || entryLess(late, early) {
		t.Error("on equal metrics the earlier submission must rank higher")
	}

	// Full ties are broken by pipeline id, so ranks never depend on map order.
	first := &LeaderboardEntry{Metric: 10.0, SubmittedAt: now, PipelineID: 1}
	second := &LeaderboardEntry{Metric: 10.0, SubmittedAt: now, PipelineID: 2}
	if !entryLess(first, second) || entryLess(second, first) {
		t.Error("full ties must be broken by pipeline id")
	}
}

func TestFillLeaderboards(t *testing.T) {
	deadline := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	currentDeadlines := &deadlines.Deadlines{Assignments: []deadlines.TaskGroup{{
		Deadline: deadlines.Date{Time: deadline},
		Tasks: []deadlines.Task{
			{Task: "bench", Score: 100, Leaderboard: &deadlines.LeaderboardSpec{Bonus: 1}},
			{Task: "plain", Score: 100},
		},
	}}}
	boards := newLeaderboards(currentDeadlines)
	if len(boards) != 1 || boards["bench"] == nil {
		t.Fatalf("unexpected boards: %#v", boards)
	}

	fillLeaderboards(boards, []database.GroupBenchmark{
		{GitlabLogin: "alice", FirstName: "Alice", LastName: "A", Task: "bench", PipelineID: 1, Metric: 3.0, SubmittedAt: deadline.Add(-2 * time.Hour)},
		{GitlabLogin: "alice", FirstName: "Alice", LastName: "A", Task: "bench", PipelineID: 2, Metric: 1.0, SubmittedAt: deadline.Add(-time.Hour)},
		{GitlabLogin: "bob", FirstName: "Bob", LastName: "B", Task: "bench", PipelineID: 3, Metric: 2.0, SubmittedAt: deadline.Add(-time.Hour)},
		// Past the deadline: does not count even though it is the best result.
		{GitlabLogin: "bob", FirstName: "Bob", LastName: "B", Task: "bench", PipelineID: 4, Metric: 0.5, SubmittedAt: deadline.Add(time.Hour)},
		// Not a leaderboard task.
		{GitlabLogin: "carol", FirstName: "Carol", LastName: "C", Task: "plain", PipelineID: 5, Metric: 0.1, SubmittedAt: deadline.Add(-time.Hour)},
	})

	board := boards["bench"]
	if len(board.Entries) != 2 || board.Entries[0].PipelineID != 2 || board.Entries[1].PipelineID != 3 || board.Entries[0].Name != "Alice A" {
		t.Fatalf("unexpected entries: %+v", board.Entries)
	}
	if rank, ok := board.Rank("bob"); !ok || rank != 2 {
		t.Fatalf("bob rank = %d, found = %v; want 2", rank, ok)
	}
	if _, ok := board.Rank("carol"); ok {
		t.Fatal("carol has no benchmark result")
	}
}

// TestLeaderboardBonusOnlyByDeadline: a student ranked by an early benchmark
// result gets the bonus only if the submission that scores the task was also
// made by the deadline (in the merge request workflow they can differ).
func TestLeaderboardBonusOnlyByDeadline(t *testing.T) {
	login := "student"
	repo := "https://gitlab/group/project"
	user := &models.User{GitlabUser: models.GitlabUser{GitlabLogin: &login, Repository: &repo}}
	d := makeWeekDeadlines()
	d.Assignments[0].Tasks[0].Leaderboard = &deadlines.LeaderboardSpec{Bonus: 1}
	if err := d.BuildScoringGroups(); err != nil {
		t.Fatal(err)
	}
	boards := newLeaderboards(d)
	fillLeaderboards(boards, []database.GroupBenchmark{{GitlabLogin: login, Task: testTask, PipelineID: 7, Metric: 1, SubmittedAt: testDeadline.Add(-2 * time.Hour)}})

	score := func(pipelineAt time.Time, boards leaderboardsMap) ScoredTask {
		scores, err := makeMergeRequestScorer().calcUserScoresImpl(
			d, user,
			func(string) ([]models.Pipeline, error) { return nil, nil },
			func(string) ([]models.Flag, error) { return nil, nil },
			func(string) ([]models.MergeRequest, error) {
				return []models.MergeRequest{merged(1, pipelineAt, testRobot)}, nil
			},
			nil, boards,
		)
		if err != nil {
			t.Fatal(err)
		}
		return scores.Groups[0].Tasks[0]
	}

	early := score(testDeadline.Add(-time.Hour), boards)
	if early.Rank != 1 || early.Score != 2000 {
		t.Fatalf("submission by the deadline: rank %d, score %d; want rank 1, score 2000", early.Rank, early.Score)
	}
	late := score(testDeadline.Add(time.Hour), boards)
	plain := score(testDeadline.Add(time.Hour), nil)
	if late.Rank != 1 || late.Score != plain.Score || late.Score >= 1000 {
		t.Fatalf("late submission: rank %d, score %d, plain %d; want rank 1 and the plain decayed score", late.Rank, late.Score, plain.Score)
	}
}

func TestMakeLeaderboardURL(t *testing.T) {
	got := makeLeaderboardURL("jit/fastest", "group with spaces")
	want := "/leaderboard/jit/fastest?group=group+with+spaces"
	if got != want {
		t.Fatalf("unexpected leaderboard URL: got %q, want %q", got, want)
	}
}
