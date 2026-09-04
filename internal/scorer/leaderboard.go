package scorer

import (
	"math"
	"sort"
	"time"

	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
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
	Task          string
	Deadline      deadlines.Date
	Entries       []LeaderboardEntry
	BannedEntries []LeaderboardEntry

	ranks map[string]int
}

func (l *TaskLeaderboard) Rank(gitlabLogin string) (int, bool) {
	rank, found := l.ranks[gitlabLogin]
	return rank, found
}

type leaderboardsMap map[string]*TaskLeaderboard

func benchmarkLogins(users []*models.User) []string {
	seen := make(map[string]struct{}, len(users))
	logins := make([]string, 0, len(users))
	for _, user := range users {
		if user.GitlabLogin == nil {
			continue
		}
		if _, found := seen[*user.GitlabLogin]; found {
			continue
		}
		seen[*user.GitlabLogin] = struct{}{}
		logins = append(logins, *user.GitlabLogin)
	}
	return logins
}

// CalcLeaderboards builds leaderboards for every benchmark task of the given
// deadlines. Only results from the provided group users and submitted before
// the task group deadline count.
func (s Scorer) CalcLeaderboards(currentDeadlines *deadlines.Deadlines, users []*models.User) (map[string]*TaskLeaderboard, error) {
	empty := calcLeaderboardsFromResults(currentDeadlines, nil, nil)
	if len(empty) == 0 {
		return empty, nil
	}
	results, err := s.db.ListBenchmarksForLogins(benchmarkLogins(users))
	if err != nil {
		return nil, err
	}
	bans, err := s.loadSubmissionBans()
	if err != nil {
		return nil, err
	}
	return calcLeaderboardsFromResults(currentDeadlines, results, bans), nil
}

func calcLeaderboardsFromResults(currentDeadlines *deadlines.Deadlines, results []models.BenchmarkResult, bans submissionBans) leaderboardsMap {
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
		return boards
	}

	best := make(map[string]map[string]*LeaderboardEntry)
	for i := range results {
		result := &results[i]
		board, found := boards[result.Task]
		if !found || result.CreatedAt.After(board.Deadline.Time) {
			continue
		}
		entry := &LeaderboardEntry{
			GitlabLogin: result.GitlabLogin,
			Metric:      result.Metric,
			SubmittedAt: result.CreatedAt,
			PipelineID:  result.PipelineID,
		}
		if _, banned := bans[result.PipelineID]; banned {
			board.BannedEntries = append(board.BannedEntries, *entry)
			continue
		}
		entriesByLogin, found := best[result.Task]
		if !found {
			entriesByLogin = make(map[string]*LeaderboardEntry)
			best[result.Task] = entriesByLogin
		}
		prev, found := entriesByLogin[result.GitlabLogin]
		if !found || entryLess(entry, prev) {
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
	for _, board := range boards {
		sort.Slice(board.BannedEntries, func(i, j int) bool {
			return board.BannedEntries[i].SubmittedAt.After(board.BannedEntries[j].SubmittedAt)
		})
	}
	return boards
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
