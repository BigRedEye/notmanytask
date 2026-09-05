package scorer

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/pkg/errors"
)

type ProjectNameFactory interface {
	MakeProjectURL(user *models.User) string
	MakePipelineURL(user *models.User, pipeline *models.Pipeline) string
	MakeBranchURL(user *models.User, pipeline *models.Pipeline) string
	MakeMergeRequestURL(user *models.User, mergeRequest *models.MergeRequest) string
	MakeTaskURL(task string) string
}

type Scorer struct {
	deadlines *deadlines.Fetcher
	db        *database.DataBase
	projects  ProjectNameFactory
	// mergeRequests is nil in the pipeline workflow
	mergeRequests *config.MergeRequestsConfig
	now           func() time.Time
}

func NewScorer(db *database.DataBase, deadlines *deadlines.Fetcher, projects ProjectNameFactory, mergeRequests *config.MergeRequestsConfig) *Scorer {
	return &Scorer{deadlines, db, projects, mergeRequests, time.Now}
}

const (
	taskStatusBanned = iota
	taskStatusAssigned
	taskStatusFailed
	taskStatusChecking
	taskStatusSuccess
)

type taskStatus = int

func classifyPipelineStatus(status models.PipelineStatus) taskStatus {
	switch status {
	case models.PipelineStatusBanned:
		return taskStatusBanned
	case models.PipelineStatusFailed:
		return taskStatusFailed
	case models.PipelineStatusCanceled:
		return taskStatusFailed
	case models.PipelineStatusPending:
		return taskStatusChecking
	case models.PipelineStatusRunning:
		return taskStatusChecking
	case models.PipelineStatusSuccess:
		return taskStatusSuccess
	default:
		return taskStatusAssigned
	}
}

// pipelineLess picks the pipeline that represents a task: the best status
// wins; among successful ones the earliest (it sets the score), among the
// others the latest (it is the one the student is working on).
func pipelineLess(left *models.Pipeline, right *models.Pipeline) bool {
	if classifyPipelineStatus(left.Status) == classifyPipelineStatus(right.Status) {
		if left.Status == models.PipelineStatusSuccess {
			return left.StartedAt.Before(right.StartedAt)
		}
		return left.StartedAt.After(right.StartedAt)
	}

	return classifyPipelineStatus(left.Status) > classifyPipelineStatus(right.Status)
}

// TODO(BigRedEye): Unify submits?
type pipelinesMap map[string]*models.Pipeline
type flagsMap map[string]*models.Flag

type pipelinesProvider = func(project string) (pipelines []models.Pipeline, err error)
type flagsProvider = func(gitlabLogin string) (flags []models.Flag, err error)
type mergeRequestsProvider = func(project string) (mergeRequests []models.MergeRequest, err error)

func (s Scorer) loadUserPipelines(user *models.User, provider pipelinesProvider) (pipelinesMap, error) {
	pipelines, err := provider(user.GetProjectName())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list use rpipelines")
	}

	pipelinesMap := make(pipelinesMap)
	for i := range pipelines {
		pipeline := &pipelines[i]
		prev, found := pipelinesMap[pipeline.Task]
		if !found || pipelineLess(pipeline, prev) {
			prev = pipeline
		}
		pipelinesMap[pipeline.Task] = prev
	}
	return pipelinesMap, nil
}

func (s Scorer) loadUserFlags(user *models.User, provider flagsProvider) (flagsMap, error) {
	flags, err := provider(*user.GitlabLogin)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list user flags")
	}

	flagsMap := make(flagsMap)
	for i := range flags {
		flag := &flags[i]
		prev, found := flagsMap[flag.Task]
		if !found || flag.CreatedAt.Before(prev.CreatedAt) {
			prev = flag
		}
		flagsMap[flag.Task] = prev
	}
	return flagsMap, nil
}

func copyDeadlines(src *deadlines.Deadlines) *deadlines.Deadlines {
	dst := *src
	dst.Assignments = make([]deadlines.TaskGroup, len(src.Assignments))
	copy(dst.Assignments, src.Assignments)
	return &dst
}

type UserFilter = func(user *models.User) bool

func (s Scorer) CalcScoreboard(groupName string) (*Standings, error) {
	return s.CalcScoreboardWithFilter(groupName, nil)
}

func (s Scorer) CalcScoreboardWithFilter(groupName string, filter UserFilter) (*Standings, error) {
	currentDeadlines := s.deadlines.GroupDeadlines(groupName)
	if currentDeadlines == nil {
		return nil, fmt.Errorf("no deadlines found")
	}

	users, err := s.db.ListGroupUsers(groupName)
	if err != nil {
		return nil, err
	}

	if filter != nil {
		allUsers := users
		users = make([]*models.User, 0, len(allUsers))
		for _, user := range allUsers {
			if filter(user) {
				users = append(users, user)
			}
		}
	}

	pipelines, err := s.makeCachedPipelinesProvider()
	if err != nil {
		return nil, err
	}

	flags, err := s.makeCachedFlagsProvider()
	if err != nil {
		return nil, err
	}

	mergeRequests, err := s.makeCachedMergeRequestsProvider()
	if err != nil {
		return nil, err
	}

	overrides, err := s.db.ListOverrides()
	if err != nil {
		return nil, fmt.Errorf("failed to list all overrides: %w", err)
	}

	scores := make([]*UserScores, len(users))
	for i, user := range users {
		userScores, err := s.calcUserScoresImpl(currentDeadlines, user, pipelines, flags, mergeRequests, overrides)
		if err != nil {
			return nil, err
		}

		scores[i] = userScores
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].FinalMark != scores[j].FinalMark {
			return scores[i].FinalMark > scores[j].FinalMark
		}
		return scores[i].User.FullName() < scores[j].User.FullName()
	})

	return &Standings{copyDeadlines(currentDeadlines), scores}, nil
}

func (s Scorer) makeCachedPipelinesProvider() (pipelinesProvider, error) {
	pipelines, err := s.db.ListAllPipelines()
	if err != nil {
		return nil, err
	}

	pipelinesMap := make(map[string][]models.Pipeline)
	for _, pipeline := range pipelines {
		prev, found := pipelinesMap[pipeline.Project]
		if !found {
			prev = make([]models.Pipeline, 0, 1)
		}
		prev = append(prev, pipeline)
		pipelinesMap[pipeline.Project] = prev
	}

	return func(project string) (pipelines []models.Pipeline, err error) {
		return pipelinesMap[project], nil
	}, nil
}

func (s Scorer) makeCachedFlagsProvider() (flagsProvider, error) {
	flags, err := s.db.ListSubmittedFlags()
	if err != nil {
		return nil, err
	}

	flagsMap := make(map[string][]models.Flag)
	for _, flag := range flags {
		prev, found := flagsMap[*flag.GitlabLogin]
		if !found {
			prev = make([]models.Flag, 0, 1)
		}
		prev = append(prev, flag)
		flagsMap[*flag.GitlabLogin] = prev
	}

	return func(gitlabLogin string) (flags []models.Flag, err error) {
		return flagsMap[gitlabLogin], nil
	}, nil
}

func (s Scorer) makeCachedMergeRequestsProvider() (mergeRequestsProvider, error) {
	if s.mergeRequests == nil {
		return nil, nil
	}

	mergeRequests, err := s.db.ListAllMergeRequests()
	if err != nil {
		return nil, err
	}

	mergeRequestsMap := make(map[string][]models.MergeRequest)
	for _, mergeRequest := range mergeRequests {
		mergeRequestsMap[mergeRequest.Project] = append(mergeRequestsMap[mergeRequest.Project], mergeRequest)
	}

	return func(project string) (mergeRequests []models.MergeRequest, err error) {
		return mergeRequestsMap[project], nil
	}, nil
}

func (s Scorer) CalcUserScores(user *models.User) (*UserScores, error) {
	currentDeadlines := s.deadlines.GroupDeadlines(user.GroupName)
	if currentDeadlines == nil {
		return nil, fmt.Errorf("no deadlines found")
	}

	overrides, err := s.db.ListUserOverrides(*user.GitlabLogin)
	if err != nil {
		return nil, fmt.Errorf("failed to list user overrides: %w", err)
	}

	var mergeRequests mergeRequestsProvider
	if s.mergeRequests != nil {
		mergeRequests = s.db.ListProjectMergeRequests
	}

	return s.calcUserScoresImpl(currentDeadlines, user, s.db.ListProjectPipelines, s.db.ListUserFlags, mergeRequests, overrides)
}

type overrideKey struct {
	login string
	task  string
}

func parseOverrides(overrides []models.OverriddenScore) (result map[overrideKey]*models.OverriddenScore) {
	result = make(map[overrideKey]*models.OverriddenScore)
	for i := range overrides {
		result[overrideKey{
			login: overrides[i].GitlabLogin,
			task:  overrides[i].Task,
		}] = &overrides[i]
	}
	return
}

func (s Scorer) calcUserScoresImpl(
	currentDeadlines *deadlines.Deadlines,
	user *models.User,
	pipelinesP pipelinesProvider,
	flagsP flagsProvider,
	mergeRequestsP mergeRequestsProvider,
	rawOverrides []models.OverriddenScore,
) (*UserScores, error) {
	pipelinesMap, err := s.loadUserPipelines(user, pipelinesP)
	if err != nil {
		return nil, err
	}

	flagsMap, err := s.loadUserFlags(user, flagsP)
	if err != nil {
		return nil, err
	}

	var mergeRequestsMap mergeRequestsMap
	if mergeRequestsP != nil {
		mergeRequestsMap, err = s.loadUserMergeRequests(user, mergeRequestsP)
		if err != nil {
			return nil, err
		}
	}

	overrides := parseOverrides(rawOverrides)

	scores := &UserScores{
		Groups:    make([]ScoredTaskGroup, 0),
		Score:     0,
		MaxScore:  0,
		FinalMark: 0.0,
		User: User{
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			Email:         user.Email,
			Group:         user.GroupName,
			Subgroup:      user.SubgroupName,
			GitlabLogin:   *user.GitlabLogin,
			GitlabProject: user.GetProjectName(),
		},
	}

	for _, group := range currentDeadlines.Assignments {
		tasks := make([]ScoredTask, len(group.Tasks))
		totalScore := 0
		maxTotalScore := 0

		scoringGroup := currentDeadlines.GetScoringGroup(&group)
		policy := currentDeadlines.GetScoringPolicy(&group)

		for i, task := range group.Tasks {
			tasks[i] = ScoredTask{
				Task:      task.Task,
				ShortName: makeShortTaskName(task.Task),
				Status:    TaskStatusAssigned,
				Score:     0,
				MaxScore:  task.Score,
				TaskUrl:   s.projects.MakeTaskURL(task.Task),
			}
			maxTotalScore += tasks[i].MaxScore

			flag, found := flagsMap[task.Task]
			if found {
				tasks[i].Status = TaskStatusSuccess

				// FIXME(BigRedEye): I just want to sleep
				// Do not try to mimic pipelines
				tasks[i].Score = s.scorePipeline(policy, currentDeadlines, user, &task, &group, &models.Pipeline{
					StartedAt: flag.CreatedAt,
					Status:    models.PipelineStatusSuccess,
				})
			} else if task.Crashme {
				// Only flags count
			} else if mergeRequestsP != nil {
				if mergeRequest, found := mergeRequestsMap[task.Task]; found {
					s.scoreMergeRequest(&tasks[i], policy, currentDeadlines, user, &task, &group, mergeRequest)
				}
			} else if pipeline, found := pipelinesMap[task.Task]; found {
				tasks[i].Status = ClassifyPipelineStatus(pipeline.Status)
				tasks[i].Score = s.scorePipeline(policy, currentDeadlines, user, &task, &group, pipeline)
				tasks[i].PipelineUrl = s.projects.MakePipelineURL(user, pipeline)
				tasks[i].BranchUrl = s.projects.MakeBranchURL(user, pipeline)
			}

			override, found := overrides[overrideKey{login: *user.GitlabLogin, task: task.Task}]
			if found {
				tasks[i].Score = override.Score
				tasks[i].Status = ClassifyPipelineStatus(override.Status)
				tasks[i].Overridden = true
			}

			totalScore += tasks[i].Score
		}

		scores.Groups = append(scores.Groups, ScoredTaskGroup{
			Title:       group.Title,
			PrettyTitle: prettifyTitle(group.Title),
			Deadline:    group.Deadline,
			Score:       totalScore,
			MaxScore:    maxTotalScore,
			Tasks:       tasks,
		})
		scores.Score += totalScore
		scores.MaxScore += maxTotalScore
		if scoringGroup != nil && scoringGroup.MaxScore > 0 {
			scores.FinalMark += scoringGroup.Weight * float64(totalScore) / float64(scoringGroup.MaxScore)
		}
	}

	return scores, nil
}

var re = regexp.MustCompile(`^\d+-(.*)$`)

func prettifyTitle(title string) string {
	submatches := re.FindStringSubmatch(title)
	if len(submatches) < 2 {
		return capitalizeWords(title)
	}
	return capitalizeWords(submatches[1])
}

func capitalizeWords(title string) string {
	return strings.Title(strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '/' {
			return ' '
		}
		return r
	}, title))
}

func makeShortTaskName(name string) string {
	return path.Base(name)
}

func (s Scorer) scorePipeline(
	policy deadlines.ScoringPolicy,
	deadlines *deadlines.Deadlines,
	user *models.User,
	task *deadlines.Task,
	group *deadlines.TaskGroup,
	pipeline *models.Pipeline,
) int {
	if pipeline.Status != models.PipelineStatusSuccess {
		return 0
	}
	if policy == nil {
		return -1
	}

	score := func() int {
		if deadline := deadlines.Scoring.FinalDeadline; deadline != nil {
			if pipeline.StartedAt.After(deadline.Time) {
				return 0
			}
		}
		return policy.Score(task.Score, group.Deadline.Time, pipeline.StartedAt)
	}()

	// TODO(sskvor): Support different retake policies
	if user.HasRetake {
		minScore := int(float64(task.Score) * deadlines.Scoring.RetakePenalty)
		if minScore > score {
			score = minScore
		}
	}

	return score
}

////////////////////////////////////////////////////////////////////////////////
// Merge request workflow

const (
	mergeRequestStatusClosed = iota
	mergeRequestStatusPending
	mergeRequestStatusOnReview
	mergeRequestStatusReviewResolved
	mergeRequestStatusCantBeMerged
	mergeRequestStatusHasExtraChanges
	mergeRequestStatusNoChanges
	mergeRequestStatusPipelineFailed
	// A merged request is the final state: nothing can shadow it
	mergeRequestStatusMerged
)

type mergeRequestStatus = int

func classifyMergeRequest(mergeRequest *models.MergeRequest) mergeRequestStatus {
	switch {
	case mergeRequest.State == models.MergeRequestStateClosed:
		return mergeRequestStatusClosed
	case mergeRequest.State == models.MergeRequestStateMerged:
		return mergeRequestStatusMerged
	case mergeRequest.LastPipelineStatus == models.PipelineStatusFailed:
		return mergeRequestStatusPipelineFailed
	case mergeRequest.MergeStatus == models.MergeRequestStatusCannotBeMerged:
		return mergeRequestStatusCantBeMerged
	case mergeRequest.UserNotesCount > 0 && mergeRequest.HasUnresolvedNotes:
		return mergeRequestStatusOnReview
	case mergeRequest.UserNotesCount > 0:
		return mergeRequestStatusReviewResolved
	case mergeRequest.NoChanges:
		return mergeRequestStatusNoChanges
	case mergeRequest.ExtraChanges:
		return mergeRequestStatusHasExtraChanges
	default:
		return mergeRequestStatusPending
	}
}

// isCreditable reports whether a merged request carries pipeline evidence
// that can be scored.
func isCreditable(mergeRequest *models.MergeRequest) bool {
	return mergeRequest.LastPipelineStatus == models.PipelineStatusSuccess && !mergeRequest.LastPipelineCreatedAt.IsZero()
}

// mergeRequestBetter reports whether candidate should represent the task
// instead of current: the most actionable state wins; among merged requests a
// creditable one wins, then the earliest pipeline (best score); ties keep the
// current one, and rows come ordered by id.
func mergeRequestBetter(candidate, current *models.MergeRequest) bool {
	candidateStatus, currentStatus := classifyMergeRequest(candidate), classifyMergeRequest(current)
	if candidateStatus != currentStatus {
		return candidateStatus > currentStatus
	}
	if candidateStatus != mergeRequestStatusMerged {
		return false
	}
	if isCreditable(candidate) != isCreditable(current) {
		return isCreditable(candidate)
	}
	return isCreditable(candidate) && candidate.LastPipelineCreatedAt.Before(current.LastPipelineCreatedAt)
}

type mergeRequestInfo struct {
	MergeRequest *models.MergeRequest
	// HasUserNotes is true if any merge request of the task has notes
	HasUserNotes bool
}

type mergeRequestsMap map[string]*mergeRequestInfo

// loadUserMergeRequests picks the representative merge request per task, see
// mergeRequestBetter.
func (s Scorer) loadUserMergeRequests(user *models.User, provider mergeRequestsProvider) (mergeRequestsMap, error) {
	mergeRequests, err := provider(user.GetProjectName())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list user merge requests")
	}

	result := make(mergeRequestsMap)
	for i := range mergeRequests {
		mergeRequest := &mergeRequests[i]
		prev, found := result[mergeRequest.Task]
		if !found {
			prev = &mergeRequestInfo{}
			result[mergeRequest.Task] = prev
		}
		if prev.MergeRequest == nil || mergeRequestBetter(mergeRequest, prev.MergeRequest) {
			prev.MergeRequest = mergeRequest
		}
		if mergeRequest.UserNotesCount > 0 {
			prev.HasUserNotes = true
		}
	}
	return result, nil
}

func (s Scorer) scoreMergeRequest(
	scored *ScoredTask,
	policy deadlines.ScoringPolicy,
	deadlines *deadlines.Deadlines,
	user *models.User,
	task *deadlines.Task,
	group *deadlines.TaskGroup,
	info *mergeRequestInfo,
) {
	mergeRequest := info.MergeRequest
	status := classifyMergeRequest(mergeRequest)

	scored.PipelineUrl = s.projects.MakeMergeRequestURL(user, mergeRequest)
	scored.HasReview = info.HasUserNotes ||
		status == mergeRequestStatusMerged && mergeRequest.MergeUserLogin != s.mergeRequests.RobotLogin

	switch status {
	case mergeRequestStatusMerged:
		// A merged request is scored like a successful pipeline submitted
		// when its last pipeline was created. A merged request whose last
		// pipeline is not green (a re-check with new tests failed, or a
		// hand merge of a red one) shows as failed and scores nothing.
		if isCreditable(mergeRequest) {
			scored.Status = TaskStatusSuccess
			scored.Score = s.scorePipeline(policy, deadlines, user, task, group, &models.Pipeline{
				Status:    mergeRequest.LastPipelineStatus,
				StartedAt: mergeRequest.LastPipelineCreatedAt,
			})
		} else {
			scored.Status = TaskStatusFailed
			scored.Message = "pipeline failed"
		}
	case mergeRequestStatusPipelineFailed:
		scored.Status = TaskStatusFailed
		scored.Message = "pipeline failed"
	case mergeRequestStatusHasExtraChanges:
		scored.Status = TaskStatusFailed
		scored.Message = "extra changes"
	case mergeRequestStatusNoChanges:
		scored.Status = TaskStatusFailed
		scored.Message = "no changes"
	case mergeRequestStatusCantBeMerged:
		scored.Status = TaskStatusFailed
		scored.Message = "merge conflict"
	case mergeRequestStatusOnReview:
		scored.Status = TaskStatusReviewUnresolved
	case mergeRequestStatusReviewResolved:
		scored.Status = TaskStatusReviewResolved
		scored.Message = s.timeToMerge(mergeRequest)
	case mergeRequestStatusPending:
		scored.Status = TaskStatusReviewPending
		scored.Message = s.timeToMerge(mergeRequest)
	case mergeRequestStatusClosed:
		scored.Status = TaskStatusReviewPending
	}
}

// timeToMerge tells how long until the review period is over; empty once it
// is over (the robot merges on its next pass) or when auto-merge is off.
func (s Scorer) timeToMerge(mergeRequest *models.MergeRequest) string {
	if s.mergeRequests.ReviewTtl == 0 {
		return ""
	}
	last := mergeRequest.LastPipelineCreatedAt
	if mergeRequest.LastNoteCreatedAt.After(last) {
		last = mergeRequest.LastNoteCreatedAt
	}
	left := last.Add(s.mergeRequests.ReviewTtl).Sub(s.now())
	if left <= 0 {
		return ""
	}
	return left.Round(time.Minute).String()
}
