package scorer

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/pkg/errors"
)

type ProjectNameFactory interface {
	MakeProjectUrl(user *models.User) string
	MakeProjectName(user *models.User) string
	MakePipelineUrl(user *models.User, pipeline *models.Pipeline) string
	MakeBranchUrl(user *models.User, pipeline *models.Pipeline) string
	MakeTaskUrl(task string) string
}

type Scorer struct {
	deadlines *deadlines.Fetcher
	db        *database.DataBase
	projects  ProjectNameFactory
}

func NewScorer(db *database.DataBase, deadlines *deadlines.Fetcher, projects ProjectNameFactory) *Scorer {
	return &Scorer{deadlines, db, projects}
}

const (
	taskStatusAssigned = iota
	taskStatusFailed
	taskStatusChecking
	taskStatusSuccess
)

type taskStatus = int

func classifyPipelineStatus(status models.PipelineStatus) taskStatus {
	switch status {
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

func pipelineLess(left *models.Pipeline, right *models.Pipeline) bool {
	if classifyPipelineStatus(left.Status) == classifyPipelineStatus(right.Status) {
		return left.StartedAt.Before(right.StartedAt)
	}

	return classifyPipelineStatus(left.Status) > classifyPipelineStatus(right.Status)
}

// TODO(BigRedEye): Unify submits?
type pipelinesMap map[string]*models.Pipeline
type flagsMap map[string]*models.Flag

type pipelinesProvider = func(project string) (pipelines []models.Pipeline, err error)
type flagsProvider = func(gitlabLogin string) (flags []models.Flag, err error)

func (s Scorer) loadUserPipelines(user *models.User, provider pipelinesProvider) (pipelinesMap, error) {
	pipelines, err := provider(s.projects.MakeProjectName(user))
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
	copy(dst.Assignments, src.Assignments)
	return &dst
}

func (s Scorer) CalcScoreboard(groupName string) (*Standings, error) {
	currentDeadlines := s.deadlines.GroupDeadlines(groupName)
	if currentDeadlines == nil {
		return nil, fmt.Errorf("No deadlines found")
	}

	users, err := s.db.ListGroupUsers(groupName)
	if err != nil {
		return nil, err
	}

	pipelines, err := s.makeCachedPipelinesProvider()
	if err != nil {
		return nil, err
	}

	flags, err := s.makeCachedFlagsProvider()
	if err != nil {
		return nil, err
	}

	scores := make([]*UserScores, len(users))
	for i, user := range users {
		userScores, err := s.calcUserScoresImpl(currentDeadlines, user, pipelines, flags)
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

func (s Scorer) CalcUserScores(user *models.User) (*UserScores, error) {
	currentDeadlines := s.deadlines.GroupDeadlines(user.GroupName)
	if currentDeadlines == nil {
		return nil, fmt.Errorf("No deadlines found")
	}

	return s.calcUserScoresImpl(currentDeadlines, user, s.db.ListProjectPipelines, s.db.ListUserFlags)
}

func (s Scorer) calcUserScoresImpl(currentDeadlines *deadlines.Deadlines, user *models.User, pipelinesP pipelinesProvider, flagsP flagsProvider) (*UserScores, error) {
	pipelinesMap, err := s.loadUserPipelines(user, pipelinesP)
	if err != nil {
		return nil, err
	}

	flagsMap, err := s.loadUserFlags(user, flagsP)
	if err != nil {
		return nil, err
	}

	scores := &UserScores{
		Groups:    make([]ScoredTaskGroup, 0),
		Score:     0,
		MaxScore:  0,
		FinalMark: 0.0,
		User: User{
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			GitlabLogin:   *user.GitlabLogin,
			GitlabProject: s.projects.MakeProjectName(user),
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
				TaskUrl:   s.projects.MakeTaskUrl(task.Task),
			}
			maxTotalScore += tasks[i].MaxScore

			pipeline, found := pipelinesMap[task.Task]
			if found {
				tasks[i].Status = ClassifyPipelineStatus(pipeline.Status)
				tasks[i].Score = s.scorePipeline(policy, &task, &group, pipeline)
				tasks[i].PipelineUrl = s.projects.MakePipelineUrl(user, pipeline)
				tasks[i].BranchUrl = s.projects.MakeBranchUrl(user, pipeline)
			} else {
				flag, found := flagsMap[task.Task]
				if found {
					tasks[i].Status = TaskStatusSuccess

					// FIXME(BigRedEye): I just want to sleep
					// Do not try to mimic pipelines
					tasks[i].Score = s.scorePipeline(policy, &task, &group, &models.Pipeline{
						StartedAt: flag.CreatedAt,
						Status:    models.PipelineStatusSuccess,
					})
				}
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

func (s Scorer) scorePipeline(policy deadlines.ScoringPolicy, task *deadlines.Task, group *deadlines.TaskGroup, pipeline *models.Pipeline) int {
	if pipeline.Status != models.PipelineStatusSuccess {
		return 0
	}
	if policy == nil {
		return -1
	}
	return policy.Score(task.Score, group.Deadline.Time, pipeline.StartedAt)
}
