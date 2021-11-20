package gitlab

import (
	"context"
	"strings"
	"sync"
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

	fresh sync.Map
}

func NewPipelinesFetcher(client *Client, db *database.DataBase) (*PipelinesFetcher, error) {
	return &PipelinesFetcher{
		Client: client,
		logger: client.logger.Named("pipelines"),
		db:     db,
	}, nil
}

func (p *PipelinesFetcher) Run(ctx context.Context) {
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

func (p *PipelinesFetcher) RunFresh(ctx context.Context) {
	tick := time.Tick(time.Millisecond * 100)

	for {
		select {
		case <-tick:
			p.fetchFreshPipelines()
		case <-ctx.Done():
			p.logger.Info("Stopping fresh pipelines fetcher")
			return
		}
	}
}

type qualifiedPipelineId struct {
	project string
	id      int
}

func (p *PipelinesFetcher) AddFresh(id int, project string) error {
	p.logger.Info("Added fresh pipeline", lf.ProjectName(project), lf.PipelineID(id))
	p.fresh.Store(&qualifiedPipelineId{project, id}, true)
	return nil
}

func (p *PipelinesFetcher) fetch(id int, project string) (*gitlab.PipelineInfo, error) {
	log := p.logger.With(
		lf.PipelineID(id),
		lf.ProjectName(project),
	)

	log.Debug("Fetching pipeline")

	pipeline, _, err := p.gitlab.Pipelines.GetPipeline(p.Client.MakeProjectWithNamespace(project), id)
	if err != nil {
		log.Error("Failed to fetch pipeline", zap.Error(err))
		return nil, errors.Wrap(err, "Failed to fetch pipeline")
	}

	info := &gitlab.PipelineInfo{
		ID:        pipeline.ID,
		Ref:       pipeline.Ref,
		Status:    pipeline.Status,
		CreatedAt: pipeline.CreatedAt,
		ProjectID: pipeline.ProjectID,
	}
	return info, p.addPipeline(project, info)
}

func (p *PipelinesFetcher) addPipeline(projectName string, pipeline *gitlab.PipelineInfo) error {
	return p.db.AddPipeline(&models.Pipeline{
		ID:        pipeline.ID,
		Task:      ParseTaskFromBranch(pipeline.Ref),
		Status:    pipeline.Status,
		Project:   projectName,
		StartedAt: *pipeline.CreatedAt,
	})
}

func (p *PipelinesFetcher) fetchAllPipelines() {
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

func (p *PipelinesFetcher) forEachProject(callback func(project *gitlab.Project) error) error {
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

func (p *PipelinesFetcher) fetchFreshPipelines() {
	removed := make([]interface{}, 0)
	p.fresh.Range(func(key, value interface{}) bool {
		id := key.(*qualifiedPipelineId)
		p.logger.Info("Fetching fresh pipeline", lf.ProjectName(id.project), lf.PipelineID(id.id))
		info, err := p.fetch(id.id, id.project)
		if err != nil {
			p.logger.Error("Failed to fetch pipeline", zap.Error(err))
		} else if info.Status != models.PipelineStatusRunning {
			p.logger.Info("Fetched fresh pipeline", lf.ProjectName(id.project), lf.PipelineID(id.id), lf.PipelineStatus(info.Status))
			removed = append(removed, id)
		}
		return true
	})

	for _, id := range removed {
		p.fresh.Delete(id)
	}
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
