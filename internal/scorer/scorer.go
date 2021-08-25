package scorer

import (
	"fmt"
	"time"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/pkg/errors"
)

type ProjectNameFactory interface {
	MakeProjectName(user *models.User) string
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

func (s Scorer) CalcScores(user *models.User) (*UserScores, error) {
	currentDeadlines := s.deadlines.GroupDeadlines(user.GroupName)
	if currentDeadlines == nil {
		return nil, fmt.Errorf("No deadlines found")
	}

	pipelines, err := s.db.ListProjectPipelines(s.projects.MakeProjectName(user))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list pipelines")
	}

	pipelinesMap := make(map[string][]*models.Pipeline)
	for i, pipeline := range pipelines {
		prev, found := pipelinesMap[pipeline.Task]
		if !found {
			prev = make([]*models.Pipeline, 0, 1)
		}
		prev = append(prev, &pipelines[i])
		pipelinesMap[pipeline.Task] = prev
	}

	scores := &UserScores{
		Groups: make([]ScoredTaskGroup, 0),
	}

	for _, group := range *currentDeadlines {
		tasks := make([]ScoredTask, len(group.Tasks))
		totalScore := 0
		maxTotalScore := 0

		for i, task := range group.Tasks {
			tasks[i] = ScoredTask{
				Task:     task.Task,
				Status:   TaskStatusAssigned,
				Score:    0,
				MaxScore: task.Score,
			}
			maxTotalScore += tasks[i].MaxScore

			pipelines, found := pipelinesMap[task.Task]
			if !found || len(pipelines) == 0 {
				continue
			}

			minPipeline := pipelines[0]
			for _, pipeline := range pipelines {
				if pipelineLess(pipeline, minPipeline) {
					minPipeline = pipeline
				}
			}

			tasks[i].Status = minPipeline.Status
			tasks[i].Score = s.scorePipeline(&task, &group, minPipeline)
			totalScore += tasks[i].Score
		}

		scores.Groups = append(scores.Groups, ScoredTaskGroup{
			Title:    group.Group,
			Deadline: group.Deadline,
			Score:    totalScore,
			MaxScore: maxTotalScore,
			Tasks:    tasks,
		})
	}

	return scores, nil
}

const (
	week = time.Hour * 24 * 7
)

// TODO(BigRedEye): Do not hardcode scoring logic
// Maybe read scoring model from deadlines?
func (s Scorer) scorePipeline(task *deadlines.Task, group *deadlines.TaskGroup, pipeline *models.Pipeline) int {
	if pipeline.Status != models.PipelineStatusSuccess {
		return 0
	}

	deadline := group.Deadline.Time

	if pipeline.StartedAt.Before(deadline) {
		return task.Score
	}

	weekAfter := group.Deadline.Time.Add(week)
	if pipeline.StartedAt.After(weekAfter) {
		return task.Score / 2
	}

	mult := 0.5 + 0.5*pipeline.StartedAt.Sub(deadline).Seconds()/(weekAfter.Sub(deadline)).Seconds()

	return int(float64(task.Score) * mult)
}
