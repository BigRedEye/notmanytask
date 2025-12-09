package gitlab

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/interfaces"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
)

type PipelinesFetcher struct {
	interfaces.GitHostingService

	logger *zap.Logger
	db     *database.DataBase

	fresh sync.Map

	config *config.Config
}

func NewPipelinesFetcher(githosting interfaces.GitHostingService, logger *zap.Logger, db *database.DataBase, config *config.Config) (*PipelinesFetcher, error) {
	return &PipelinesFetcher{
		GitHostingService: githosting,
		logger:            logger.Named("pipelines"),
		db:                db,
		config:            config,
	}, nil
}

func (p *PipelinesFetcher) Run(ctx context.Context) {
	interval := p.config.PullIntervals.Pipelines
	if interval == nil {
		return
	}

	tick := time.NewTicker(*interval)

	for {
		select {
		case <-tick.C:
			p.fetchAllPipelines()
		case <-ctx.Done():
			p.logger.Info("Stopping pipelines fetcher")
			return
		}
	}
}

func (p *PipelinesFetcher) RunFresh(ctx context.Context) {
	tick := time.NewTicker(time.Second)

	for {
		select {
		case <-tick.C:
			p.fetchFreshPipelines()
		case <-ctx.Done():
			p.logger.Info("Stopping fresh pipelines fetcher")
			return
		}
	}
}

type qualifiedPipelineID struct {
	project string
	id      int
}

func (p *PipelinesFetcher) AddFresh(id int, project string) error {
	p.logger.Info("Added fresh pipeline", lf.ProjectName(project), lf.PipelineID(id))
	p.fresh.Store(&qualifiedPipelineID{project, id}, true)
	return nil
}

func (p *PipelinesFetcher) fetch(id int, project string) (*gitlab.PipelineInfo, error) {
	log := p.logger.With(
		lf.PipelineID(id),
		lf.ProjectName(project),
	)

	log.Debug("Fetching pipeline")

	pipeline, err := p.GetCIRun(project, id)
	if err != nil {
		log.Error("Failed to fetch pipeline", zap.Error(err))
		return nil, errors.Wrap(err, "Failed to fetch pipeline")
	}

	info := &gitlab.PipelineInfo{
		ID:        pipeline.ID,
		Ref:       pipeline.Ref,
		Status:    pipeline.Status,
		CreatedAt: &pipeline.StartedAt,
		ProjectID: pipeline.ProjectID,
	}
	return info, p.addPipeline(project, pipeline)
}

func (p *PipelinesFetcher) addPipeline(projectName string, pipeline *models.Pipeline) error {
	return p.db.AddPipeline(pipeline)
}

func (p *PipelinesFetcher) fetchAllPipelines() {
	p.logger.Debug("Start pipelines fetcher iteration")
	defer p.logger.Debug("Finish pipelines fetcher iteration")

	err := p.forEachProject(func(project *models.Repo) error {
		p.logger.Debug("Found project", lf.ProjectName(project.Name))

		pipelines, err := p.ListRepoCIRuns(project)
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

		return nil
	})

	if err == nil {
		p.logger.Debug("Successfully fetched pipelines")
	} else {
		p.logger.Error("Failed to fetch pipelines", zap.Error(err))
	}
}

func (p *PipelinesFetcher) forEachProject(callback func(project *models.Repo) error) error {
	projects, err := p.ListAllRepos()
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

	return nil
}

func (p *PipelinesFetcher) fetchFreshPipelines() {
	removed := make([]interface{}, 0)
	p.fresh.Range(func(key, _ interface{}) bool {
		id := key.(*qualifiedPipelineID)
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
	tasksPrefix  = "tasks/"
)

func ParseTaskFromBranch(task string) string {
	return strings.TrimPrefix(strings.TrimPrefix(task, branchPrefix), tasksPrefix)
}
