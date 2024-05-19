package gitlab

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/platform/base"
)

type PipelinesFetcherGitlab struct {
	*base.PipelinesFetcherBase
	*ClientGitlab
}

func (p *PipelinesFetcherGitlab) Run(ctx context.Context) {
	interval := p.Config.PullIntervals.Pipelines
	if interval == nil {
		return
	}

	tick := time.NewTicker(*interval)

	for {
		select {
		case <-tick.C:
			p.FetchAllPipelines()
		case <-ctx.Done():
			p.Logger.Info("Stopping pipelines fetcher")
			return
		}
	}
}

func (p *PipelinesFetcherGitlab) RunFresh(ctx context.Context) {
	tick := time.NewTicker(time.Second)

	for {
		select {
		case <-tick.C:
			p.FetchFreshPipelines()
		case <-ctx.Done():
			p.Logger.Info("Stopping fresh pipelines fetcher")
			return
		}
	}
}

func (p *PipelinesFetcherGitlab) AddPipeline(projectName string, pipeline base.PipelineInfo) error {
	pipeline_gitlab, cast := pipeline.(*gitlab.PipelineInfo)
	if !cast {
		return errors.New("Failed to cast pipeline to gitlab pipeline")
	}
	return p.Db.AddPipeline(&models.Pipeline{
		ID:        pipeline_gitlab.ID,
		Task:      base.ParseTaskFromBranch(pipeline_gitlab.Ref),
		Status:    pipeline_gitlab.Status,
		Project:   projectName,
		StartedAt: *pipeline_gitlab.CreatedAt,
	})
}

func (p *PipelinesFetcherGitlab) Fetch(id int, project string) (base.PipelineInfo, error) {
	log := p.Logger.With(
		lf.PipelineID(id),
		lf.ProjectName(project),
	)

	log.Debug("Fetching pipeline")

	pipeline, _, err := p.Gitlab.Pipelines.GetPipeline(p.ClientGitlab.MakeProjectWithNamespace(project), id)
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
	return info, p.AddPipeline(project, info)
}

func (p *PipelinesFetcherGitlab) FetchAllPipelines() {
	p.Logger.Debug("Start pipelines fetcher iteration")
	defer p.Logger.Debug("Finish pipelines fetcher iteration")

	err := p.ForEachProject(func(project base.Project) error {
		project_gitlab, cast := project.(*gitlab.Project)
		if !cast {
			return errors.New("Failed to cast project to gitlab project")
		}
		p.Logger.Debug("Found project", lf.ProjectName(project_gitlab.Name))
		options := &gitlab.ListProjectPipelinesOptions{}
		for {
			pipelines, resp, err := p.Gitlab.Pipelines.ListProjectPipelines(project_gitlab.ID, options)
			if err != nil {
				p.Logger.Error("Failed to list projects", zap.Error(err))
				return err
			}

			for _, pipeline := range pipelines {
				p.Logger.Debug("Found pipeline", lf.ProjectName(project_gitlab.Name), lf.PipelineID(pipeline.ID), lf.PipelineStatus(pipeline.Status))
				if err = p.AddPipeline(project_gitlab.Name, pipeline); err != nil {
					p.Logger.Error("Failed to add pipeline", zap.Error(err), lf.ProjectName(project_gitlab.Name), lf.PipelineID(pipeline.ID))
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
		p.Logger.Debug("Successfully fetched pipelines")
	} else {
		p.Logger.Error("Failed to fetch pipelines", zap.Error(err))
	}
}

func (p *PipelinesFetcherGitlab) ForEachProject(callback func(project base.Project) error) error {
	options := gitlab.ListGroupProjectsOptions{}

	for {
		projects, resp, err := p.Gitlab.Groups.ListGroupProjects(p.Config.Platform.GitLab.Group.ID, &options)
		if err != nil {
			p.Logger.Error("Failed to list projects", zap.Error(err))
			return err
		}

		for _, project := range projects {
			if err = callback(project); err != nil {
				p.Logger.Error("Project callback failed", zap.Error(err))
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

func (p *PipelinesFetcherGitlab) FetchFreshPipelines() {
	removed := make([]interface{}, 0)
	p.Fresh.Range(func(key, _ interface{}) bool {
		id := key.(*base.QualifiedPipelineID)
		info, err := p.Fetch(id.Id, id.Project)
		info_gitlab, cast := info.(*gitlab.PipelineInfo)
		if !cast {
			p.Logger.Error("Failed to cast pipeline info to gitlab pipeline info", lf.ProjectName(id.Project), lf.PipelineID(id.Id))
		}
		if err != nil {
			p.Logger.Error("Failed to fetch pipeline", zap.Error(err))
		} else if info_gitlab.Status != models.PipelineStatusRunning {
			p.Logger.Info("Fetched fresh pipeline", lf.ProjectName(id.Project), lf.PipelineID(id.Id), lf.PipelineStatus(info_gitlab.Status))
			removed = append(removed, id)
		}
		return true
	})

	for _, id := range removed {
		p.Fresh.Delete(id)
	}
}
