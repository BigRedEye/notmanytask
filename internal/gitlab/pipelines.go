package gitlab

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/database"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
)

type PipelinesFetcher struct {
	*Client

	logger *zap.Logger
	db     *database.DataBase
}

func NewPipelinesFetcher(client *Client, db *database.DataBase) (*PipelinesFetcher, error) {
	return &PipelinesFetcher{
		Client: client,
		logger: client.logger.Named("pipelines"),
		db:     db,
	}, nil
}

func (p PipelinesFetcher) Run(ctx context.Context) {
	tick := time.Tick(p.config.PullIntervals.Pipelines)

	for {
		select {
		case <-tick:
			p.fetchAllPipelines()
		case <-ctx.Done():
			p.logger.Info("Stopping pipelines fetcher")
			return
		}
	}
}

func (p PipelinesFetcher) Fetch(id int, project string) error {
	log := p.logger.With(
		lf.PipelineID(id),
		lf.ProjectName(project),
	)

	log.Debug("Fetching pipeline")

	pipeline, _, err := p.gitlab.Pipelines.GetPipeline(p.Client.MakeProjectWithNamespace(project), id)
	if err != nil {
		log.Error("Failed to fetch pipeline", zap.Error(err))
		return errors.Wrap(err, "Failed to fetch pipeline")
	}

	return p.addPipeline(project, &gitlab.PipelineInfo{
		ID:        pipeline.ID,
		Ref:       pipeline.Ref,
		Status:    pipeline.Status,
		CreatedAt: pipeline.CreatedAt,
		ProjectID: pipeline.ProjectID,
	})
}

func (p PipelinesFetcher) addPipeline(projectName string, pipeline *gitlab.PipelineInfo) error {
	updated, err := p.db.AddPipeline(&models.Pipeline{
		ID:        pipeline.ID,
		Task:      ParseTaskFromBranch(pipeline.Ref),
		Status:    pipeline.Status,
		Project:   projectName,
		StartedAt: *pipeline.CreatedAt,
	})
	if err != nil {
		return err
	}
	if updated {
		p.logger.Info("Updated pipeline",
			lf.ProjectName(projectName),
			lf.PipelineID(pipeline.ID),
			lf.PipelineStatus(pipeline.Status),
		)
	}
	return nil
}

func (p PipelinesFetcher) fetchAllPipelines() {
	p.logger.Debug("Start pipelines fetcher iteration")
	defer p.logger.Debug("Finish pipelines fetcher iteration")

	err := p.forEachProject(func(project *gitlab.Project) error {
		p.logger.Debug("Found project", lf.ProjectName(project.Name))
		options := &gitlab.ListProjectPipelinesOptions{}
		for {
			pipelines, resp, err := p.gitlab.Pipelines.ListProjectPipelines(project.ID, options)
			if err != nil {
				p.logger.Error("Failed to list projects", zap.Error(err))
				return err
			}

			for _, pipeline := range pipelines {
				p.logger.Debug("Found pipeline", lf.ProjectName(project.Name), lf.PipelineID(pipeline.ID), lf.PipelineStatus(pipeline.Status))
				if err = p.addPipeline(project.Name, pipeline); err != nil {
					p.logger.Error("Failed to add pipeline", zap.Error(err), lf.ProjectName(project.Name), lf.PipelineID(pipeline.ID))
				}
			}

			if resp.CurrentPage >= resp.TotalPages {
				break
			}
			options.Page = resp.NextPage
		}

		return nil
	})

	if err == nil {
		p.logger.Debug("Sucessfully fetched pipelines")
	} else {
		p.logger.Error("Failed to fetch pipelines", zap.Error(err))
	}
}

func (p PipelinesFetcher) forEachProject(callback func(project *gitlab.Project) error) error {
	options := gitlab.ListGroupProjectsOptions{}

	for {
		projects, resp, err := p.gitlab.Groups.ListGroupProjects(p.config.GitLab.Group.ID, &options)
		if err != nil {
			p.logger.Error("Failed to list projects", zap.Error(err))
			return err
		}

		for _, project := range projects {
			if err = callback(project); err != nil {
				p.logger.Error("Project callback failed", zap.Error(err))
				return err
			}
		}

		if resp.CurrentPage >= resp.TotalPages {
			break
		}
		options.Page = resp.NextPage
	}

	return nil
}

const (
	branchPrefix = "submits/"
)

func ParseTaskFromBranch(task string) string {
	return strings.TrimPrefix(task, branchPrefix)
}

func MakeBranchForTask(task string) string {
	return branchPrefix + task
}
