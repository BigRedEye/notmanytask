package scorer

import (
	"math"
	"sort"
	"time"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
)

type LeaderboardEntry struct {
	GitlabLogin string
	Name        string
	Metric      float64
	SubmittedAt time.Time
	PipelineID  int
}

// TaskLeaderboard holds the best pre-deadline result of every user for one
// benchmark task. Entries are sorted best-first; rank of Entries[i] is i+1.
type TaskLeaderboard struct {
	Deadline deadlines.Date
	Entries  []LeaderboardEntry

	ranks map[string]int
}

func (l *TaskLeaderboard) Rank(gitlabLogin string) (int, bool) {
	rank, found := l.ranks[gitlabLogin]
	return rank, found
}

type leaderboardsMap map[string]*TaskLeaderboard

func newLeaderboards(currentDeadlines *deadlines.Deadlines) leaderboardsMap {
	boards := make(leaderboardsMap)
	for i := range currentDeadlines.Assignments {
		group := &currentDeadlines.Assignments[i]
		for j := range group.Tasks {
			if task := &group.Tasks[j]; task.Leaderboard != nil {
				boards[task.Task] = &TaskLeaderboard{Deadline: group.Deadline, ranks: make(map[string]int)}
			}
		}
	}
	return boards
}

// CalcLeaderboards builds leaderboards for every benchmark task of the given
// deadlines from the successful pipelines of the group's users. Only results
// submitted before the task group deadline count.
func (s Scorer) CalcLeaderboards(currentDeadlines *deadlines.Deadlines, group string) (map[string]*TaskLeaderboard, error) {
	boards := newLeaderboards(currentDeadlines)
	if len(boards) == 0 {
		return boards, nil
	}
	results, err := s.db.ListGroupBenchmarks(group)
	if err != nil {
		return nil, err
	}
	fillLeaderboards(boards, results)
	return boards, nil
}

func fillLeaderboards(boards leaderboardsMap, results []database.GroupBenchmark) {
	best := make(map[string]map[string]*LeaderboardEntry)
	for i := range results {
		result := &results[i]
		board, found := boards[result.Task]
		if !found || result.SubmittedAt.After(board.Deadline.Time) {
			continue
		}
		entry := &LeaderboardEntry{
			GitlabLogin: result.GitlabLogin,
			Name:        result.FirstName + " " + result.LastName,
			Metric:      result.Metric,
			SubmittedAt: result.SubmittedAt,
			PipelineID:  result.PipelineID,
		}
		entriesByLogin, found := best[result.Task]
		if !found {
			entriesByLogin = make(map[string]*LeaderboardEntry)
			best[result.Task] = entriesByLogin
		}
		if prev, found := entriesByLogin[result.GitlabLogin]; !found || entryLess(entry, prev) {
			entriesByLogin[result.GitlabLogin] = entry
		}
	}

	for task, entriesByLogin := range best {
		board := boards[task]
		for _, entry := range entriesByLogin {
			board.Entries = append(board.Entries, *entry)
		}
		sort.Slice(board.Entries, func(i, j int) bool {
			return entryLess(&board.Entries[i], &board.Entries[j])
		})
		for i := range board.Entries {
			board.ranks[board.Entries[i].GitlabLogin] = i + 1
		}
	}
}

// entryLess orders entries best-first: lower metric, then earlier
// submission, then pipeline id so that equal results rank deterministically.
func entryLess(left, right *LeaderboardEntry) bool {
	if left.Metric != right.Metric {
		return left.Metric < right.Metric
	}
	if !left.SubmittedAt.Equal(right.SubmittedAt) {
		return left.SubmittedAt.Before(right.SubmittedAt)
	}
	return left.PipelineID < right.PipelineID
}

// leaderboardScore scales the score of a ranked task: the first place gets
// base*(1+bonus), the last place exactly base, places in between linearly.
// base is the policy score of a submission made by the deadline (the caller
// checks that), so normally the full task score.
func leaderboardScore(base int, bonus float64, rank, total int) int {
	if rank <= 0 || total <= 0 {
		return base
	}
	fraction := 1.0
	if total > 1 {
		fraction = float64(total-rank) / float64(total-1)
	}
	return int(math.Round(float64(base) * (1.0 + bonus*fraction)))
}
