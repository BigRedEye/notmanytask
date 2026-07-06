package scorer

import (
	"testing"
	"time"
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
