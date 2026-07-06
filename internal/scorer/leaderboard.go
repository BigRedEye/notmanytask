package scorer

import (
	"math"
	"sort"
	"time"

	"github.com/bigredeye/notmanytask/internal/deadlines"
)

type LeaderboardEntry struct {
	GitlabLogin string
	Metric      float64
	SubmittedAt time.Time
	PipelineID  int
}

// TaskLeaderboard holds the best pre-deadline result of every user for one
// benchmark task. Entries are sorted best-first; rank of Entries[i] is i+1.
// Ties are broken by submission time (earlier wins).
type TaskLeaderboard struct {
	Task     string
	Deadline deadlines.Date
	Entries  []LeaderboardEntry

	ranks map[string]int
}

func (l *TaskLeaderboard) Rank(gitlabLogin string) (int, bool) {
	rank, found := l.ranks[gitlabLogin]
	return rank, found
}

type leaderboardsMap map[string]*TaskLeaderboard

// CalcLeaderboards builds leaderboards for every benchmark task of the given
// deadlines. Only results submitted before the task group deadline count.
func (s Scorer) CalcLeaderboards(currentDeadlines *deadlines.Deadlines) (map[string]*TaskLeaderboard, error) {
	boards := make(leaderboardsMap)
	for i := range currentDeadlines.Assignments {
		group := &currentDeadlines.Assignments[i]
		for j := range group.Tasks {
			task := &group.Tasks[j]
			if task.Leaderboard != nil {
				boards[task.Task] = &TaskLeaderboard{
					Task:     task.Task,
					Deadline: group.Deadline,
					ranks:    make(map[string]int),
				}
			}
		}
	}
	if len(boards) == 0 {
		return boards, nil
	}

	results, err := s.db.ListAllBenchmarks()
	if err != nil {
		return nil, err
	}

	best := make(map[string]map[string]*LeaderboardEntry)
	for i := range results {
		result := &results[i]
		board, found := boards[result.Task]
		if !found || result.CreatedAt.After(board.Deadline.Time) {
			continue
		}
		users, found := best[result.Task]
		if !found {
			users = make(map[string]*LeaderboardEntry)
			best[result.Task] = users
		}
		entry := &LeaderboardEntry{
			GitlabLogin: result.GitlabLogin,
			Metric:      result.Metric,
			SubmittedAt: result.CreatedAt,
			PipelineID:  result.PipelineID,
		}
		prev, found := users[result.GitlabLogin]
		if !found || entryLess(entry, prev) {
			users[result.GitlabLogin] = entry
		}
	}

	for task, users := range best {
		board := boards[task]
		for _, entry := range users {
			board.Entries = append(board.Entries, *entry)
		}
		sort.Slice(board.Entries, func(i, j int) bool {
			return entryLess(&board.Entries[i], &board.Entries[j])
		})
		for i := range board.Entries {
			board.ranks[board.Entries[i].GitlabLogin] = i + 1
		}
	}
	return boards, nil
}

func entryLess(left, right *LeaderboardEntry) bool {
	if left.Metric != right.Metric {
		return left.Metric < right.Metric
	}
	return left.SubmittedAt.Before(right.SubmittedAt)
}

// A correct solution always keeps its base score; the leaderboard only adds
// a bonus on top: the first place gets base*(1+bonus), the last place gets
// exactly the base score.
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
