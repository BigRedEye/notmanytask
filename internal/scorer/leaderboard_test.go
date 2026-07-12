package scorer

import (
	"testing"
	"time"

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
}

func TestBenchmarkLogins(t *testing.T) {
	alice := "alice"
	bob := "bob"
	users := []*models.User{
		{GitlabUser: models.GitlabUser{GitlabLogin: &alice}},
		{GitlabUser: models.GitlabUser{GitlabLogin: &bob}},
		{GitlabUser: models.GitlabUser{GitlabLogin: &alice}},
		{},
	}

	logins := benchmarkLogins(users)
	if len(logins) != 2 || logins[0] != "alice" || logins[1] != "bob" {
		t.Fatalf("unexpected logins: %#v", logins)
	}
}

func TestMakeLeaderboardURL(t *testing.T) {
	got := makeLeaderboardURL("jit/fastest", "group with spaces")
	want := "/leaderboard/jit/fastest?group=group+with+spaces"
	if got != want {
		t.Fatalf("unexpected leaderboard URL: got %q, want %q", got, want)
	}
}

func TestBannedBenchmarkIsExcludedAndRanksAreRecomputed(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	currentDeadlines := &deadlines.Deadlines{Assignments: []deadlines.TaskGroup{{
		Deadline: deadlines.Date{Time: deadline},
		Tasks: []deadlines.Task{{
			Task:        "bench",
			Score:       100,
			Leaderboard: &deadlines.LeaderboardSpec{Bonus: 1},
		}},
	}}}
	results := []models.BenchmarkResult{
		{GitlabLogin: "banned-fastest", Task: "bench", PipelineID: 1, Metric: 1, CreatedAt: deadline.Add(-3 * time.Minute)},
		{GitlabLogin: "alice", Task: "bench", PipelineID: 2, Metric: 2, CreatedAt: deadline.Add(-2 * time.Minute)},
		{GitlabLogin: "bob", Task: "bench", PipelineID: 3, Metric: 3, CreatedAt: deadline.Add(-time.Minute)},
	}
	bans := submissionBans{1: {PipelineID: 1}}

	board := calcLeaderboardsFromResults(currentDeadlines, results, bans)["bench"]
	if len(board.Entries) != 2 || board.Entries[0].GitlabLogin != "alice" || board.Entries[1].GitlabLogin != "bob" {
		t.Fatalf("unexpected ranked entries: %#v", board.Entries)
	}
	if rank, found := board.Rank("alice"); !found || rank != 1 {
		t.Fatalf("alice rank was not recomputed: rank=%d found=%v", rank, found)
	}
	if _, found := board.Rank("banned-fastest"); found {
		t.Fatal("banned entry still has a rank")
	}
	if len(board.BannedEntries) != 1 || board.BannedEntries[0].PipelineID != 1 {
		t.Fatalf("banned entry is not retained for UI: %#v", board.BannedEntries)
	}
}
