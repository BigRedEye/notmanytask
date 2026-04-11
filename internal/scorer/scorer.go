package scorer

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/pkg/errors"
)

type projectNameFactory interface {
	MakeProjectName(user *models.User) string
	MakeMergeRequestUrl(user *models.User, mergeRequest *models.MergeRequest) string
	MakeTaskUrl(task string) string
}

type Scorer struct {
	deadlines  *deadlines.Fetcher
	db         *database.DataBase
	projects   projectNameFactory
	reviewTtl  time.Duration
	robotLogin string
}

type MergeRequestsInfo struct {
	MergeRequest *models.MergeRequest
	HasUserNotes bool
}

func NewScorer(db *database.DataBase, deadlines *deadlines.Fetcher, projects projectNameFactory, reviewTtl time.Duration, robotLogin string) *Scorer {
	return &Scorer{deadlines, db, projects, reviewTtl, robotLogin}
}

const (
	mergeRequestStatusClosed = iota
	mergeRequestStatusPending
	mergeRequestStatusOnReview
	mergeRequestStatusReviewResolved
	mergeRequestStatusCantBeMerged
	mergeRequestStatusMerged
	mergeRequestStatusHasExtraChanges
	mergeRequestStatusPipelineFailed
)

type mergeRequestStatus = int

func getMergeRequestStatus(mergeRequest *models.MergeRequest) mergeRequestStatus {
	if mergeRequest.State == models.MergeRequestStateClosed {
		return mergeRequestStatusClosed
	} else if mergeRequest.State == models.MergeRequestStateMerged {
		return mergeRequestStatusMerged
	} else if mergeRequest.LastPipelineStatus == models.PipelineStatusFailed {
		return mergeRequestStatusPipelineFailed
	} else if mergeRequest.MergeStatus == models.MergeRequestStatusCannotBeMerged {
		return mergeRequestStatusCantBeMerged
	} else if mergeRequest.UserNotesCount > 0 {
		if mergeRequest.HasUnresolvedNotes {
			return mergeRequestStatusOnReview
		} else {
			return mergeRequestStatusReviewResolved
		}
	} else if mergeRequest.ExtraChanges {
		return mergeRequestStatusHasExtraChanges
	} else {
		return mergeRequestStatusPending
	}
}

// TODO(BigRedEye): Unify submits?
type mergeRequestsMap map[string]*MergeRequestsInfo

type mergeRequestsProvider = func(project string) (mergeRequests []models.MergeRequest, err error)

func (s Scorer) loadUserMergeRequests(user *models.User, provider mergeRequestsProvider) (mergeRequestsMap, error) {
	mergeRequests, err := provider(s.projects.MakeProjectName(user))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list user merge requests")
	}

	mergeRequestsMap := make(mergeRequestsMap)
	for i := range mergeRequests {
		mergeRequest := &mergeRequests[i]
		prev, found := mergeRequestsMap[mergeRequest.Task]
		if !found || getMergeRequestStatus(mergeRequest) > getMergeRequestStatus(prev.MergeRequest) {
			prev = &MergeRequestsInfo{
				MergeRequest: mergeRequest,
			}
		}
		if mergeRequest.UserNotesCount > 0 {
			prev.HasUserNotes = true
		}
		mergeRequestsMap[mergeRequest.Task] = prev
	}
	return mergeRequestsMap, nil
}

func (s Scorer) CalcScoreboard(groupName string, subgroupName string) (*Standings, error) {
	currentDeadlines := s.deadlines.GroupDeadlines(groupName)
	if currentDeadlines == nil {
		return nil, fmt.Errorf("No deadlines found")
	}

	users, err := s.db.ListGroupUsers(groupName, subgroupName)
	if err != nil {
		return nil, err
	}

	mergeRequests, err := s.makeCachedMergeRequestsProvider()
	if err != nil {
		return nil, err
	}

	scores := make([]*UserScores, len(users))
	for i, user := range users {
		userScores, err := s.calcUserScoresImpl(currentDeadlines, user, mergeRequests)
		if err != nil {
			return nil, err
		}

		scores[i] = userScores
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		if scores[i].TasksOnReview != scores[j].TasksOnReview {
			return scores[i].TasksOnReview > scores[j].TasksOnReview
		}
		return scores[i].User.FullName() < scores[j].User.FullName()
	})

	return &Standings{currentDeadlines, scores}, nil
}

func (s Scorer) makeCachedMergeRequestsProvider() (mergeRequestsProvider, error) {
	mergeRequests, err := s.db.ListAllMergeRequests()
	if err != nil {
		return nil, err
	}

	mergeRequestsMap := make(map[string][]models.MergeRequest)
	for _, mergeRequest := range mergeRequests {
		prev, found := mergeRequestsMap[mergeRequest.Project]
		if !found {
			prev = make([]models.MergeRequest, 0, 1)
		}
		prev = append(prev, mergeRequest)
		mergeRequestsMap[mergeRequest.Project] = prev
	}

	return func(project string) (mergeRequests []models.MergeRequest, err error) {
		return mergeRequestsMap[project], nil
	}, nil
}

func (s Scorer) CalcUserScores(user *models.User) (*UserScores, error) {
	currentDeadlines := s.deadlines.GroupDeadlines(user.GroupName)
	if currentDeadlines == nil {
		return nil, fmt.Errorf("No deadlines found")
	}

	return s.calcUserScoresImpl(currentDeadlines, user, s.db.ListProjectMergeRequests)
}

func MaxTime(x, y time.Time) time.Time {
	if x.Unix() < y.Unix() {
		return y
	}
	return x
}

func (s Scorer) calcUserScoresImpl(currentDeadlines *deadlines.Deadlines, user *models.User, mergeRequestsP mergeRequestsProvider) (*UserScores, error) {
	mergeRequestsMap, err := s.loadUserMergeRequests(user, mergeRequestsP)
	if err != nil {
		return nil, err
	}

	scores := &UserScores{
		Groups:   make([]ScoredTaskGroup, 0),
		Score:    0,
		MaxScore: 0,
		User: User{
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			Email:         user.Email,
			Group:         user.GroupName,
			Subgroup:      user.SubgroupName,
			GitlabLogin:   *user.GitlabLogin,
			GitlabProject: s.projects.MakeProjectName(user),
		},
	}

	for _, group := range *currentDeadlines {
		tasks := make([]ScoredTask, len(group.Tasks))
		totalScore := 0
		maxTotalScore := 0
		tasksOnReview := 0

		for i, task := range group.Tasks {
			tasks[i] = ScoredTask{
				Task:      task.Task,
				ShortName: makeShortTaskName(task.Task),
				Status:    TaskStatusAssigned,
				Score:     0,
				MaxScore:  task.Score,
				Message:   "",
				TaskUrl:   s.projects.MakeTaskUrl(task.Task),
			}
			maxTotalScore += tasks[i].MaxScore

			mergeRequestsInfo, mergeRequestFound := mergeRequestsMap[task.Task]
			if mergeRequestFound {
				mrStatus := getMergeRequestStatus(mergeRequestsInfo.MergeRequest)

				tasks[i].PipelineUrl = s.projects.MakeMergeRequestUrl(user, mergeRequestsInfo.MergeRequest)
				tasks[i].HasReview = mergeRequestsInfo.HasUserNotes ||
					mrStatus == mergeRequestStatusMerged && mergeRequestsInfo.MergeRequest.MergeUserLogin != s.robotLogin

				timeToMerge := fmt.Sprintf("%s", MaxTime(
					mergeRequestsInfo.MergeRequest.LastPipelineCreatedAt,
					mergeRequestsInfo.MergeRequest.LastNoteCreatedAt,
				).Add(s.reviewTtl).Sub(time.Now()).Round(time.Minute))

				switch mrStatus {
				case mergeRequestStatusOnReview:
					tasks[i].Status = TaskStatusOnReview
				case mergeRequestStatusPending:
					tasks[i].Message = timeToMerge
					tasks[i].Status = TaskStatusPending
				case mergeRequestStatusClosed:
					tasks[i].Status = TaskStatusPending
				case mergeRequestStatusCantBeMerged:
					tasks[i].Message = "merge conflict"
					tasks[i].Status = TaskStatusFailed
				case mergeRequestStatusReviewResolved:
					tasks[i].Message = timeToMerge
					tasks[i].Status = TaskStatusReviewResolved
				case mergeRequestStatusHasExtraChanges:
					tasks[i].Message = "extra changes"
					tasks[i].Status = TaskStatusFailed
				case mergeRequestStatusPipelineFailed:
					tasks[i].Message = "pipeline failed"
					tasks[i].Status = TaskStatusFailed
				case mergeRequestStatusMerged:
					tasks[i].Score = s.scorePipeline(&task, &group, mergeRequestsInfo.MergeRequest)
					tasks[i].Status = TaskStatusSuccess
				}

				if tasks[i].Status != TaskStatusFailed {
					tasksOnReview++
				}
			}

			totalScore += tasks[i].Score
		}

		scores.Groups = append(scores.Groups, ScoredTaskGroup{
			Title:       group.Group,
			PrettyTitle: prettifyTitle(group.Group),
			Deadline:    group.Deadline,
			Score:       totalScore,
			MaxScore:    maxTotalScore,
			Tasks:       tasks,
		})
		scores.Score += totalScore
		scores.MaxScore += maxTotalScore
		scores.TasksOnReview += tasksOnReview
	}

	return scores, nil
}

var re = regexp.MustCompile(`^\d+-(.*)$`)

func prettifyTitle(title string) string {
	submatches := re.FindStringSubmatch(title)
	if len(submatches) < 2 {
		return capitalize(title)
	}
	return capitalize(submatches[1])
}

func capitalize(title string) string {
	return strings.Title(title)
}

func makeShortTaskName(name string) string {
	return path.Base(name)
}

const (
	week = time.Hour * 24 * 7
)

// TODO(BigRedEye): Do not hardcode scoring logic
// Maybe read scoring model from deadlines?
type scoringFunc = func(task *deadlines.Task, group *deadlines.TaskGroup, pipeline *models.Pipeline) int

func linearScore(task *deadlines.Task, group *deadlines.TaskGroup, mergeRequest *models.MergeRequest) int {
	if mergeRequest.LastPipelineStatus != models.PipelineStatusSuccess {
		return 0
	}

	deadline := group.Deadline.Time

	if mergeRequest.LastPipelineCreatedAt.Before(deadline) {
		return task.Score
	}

	weekAfter := group.Deadline.Time.Add(week)
	if mergeRequest.LastPipelineCreatedAt.After(weekAfter) {
		return 0
	}

	mult := 1.0 - mergeRequest.LastPipelineCreatedAt.Sub(deadline).Seconds()/(weekAfter.Sub(deadline)).Seconds()

	return int(float64(task.Score) * mult)
}

func exponentialScore(task *deadlines.Task, group *deadlines.TaskGroup, mergeRequest *models.MergeRequest) int {
	if mergeRequest.LastPipelineStatus != models.PipelineStatusSuccess {
		return 0
	}

	deadline := group.Deadline.Time
	if mergeRequest.LastPipelineCreatedAt.Before(deadline) {
		return task.Score
	}

	deltaDays := mergeRequest.LastPipelineCreatedAt.Sub(deadline).Hours() / 24.0

	return int(math.Max(0.3, 1.0/math.Exp(deltaDays/5.0)) * float64(task.Score))
}

func (s Scorer) scorePipeline(task *deadlines.Task, group *deadlines.TaskGroup, mergeRequest *models.MergeRequest) int {
	return linearScore(task, group, mergeRequest)
	// return exponentialScore(task, group, pipeline)
}
