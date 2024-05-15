package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/platform/base"
)

type PipelinesInfoResponse struct {
	TotalCount   int64               `json:"total_count"`
	WorkflowRuns []PipelineInfoGitea `json:"workflow_runs"`
}

type PipelineInfoGitea struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	HeadBranch   string     `json:"head_branch"`
	HeadSHA      string     `json:"head_sha"`
	RunNumber    int        `json:"run_number"`
	Event        string     `json:"event"`
	DisplayTitle string     `json:"display_title"`
	Status       string     `json:"status"`
	WorkflowID   string     `json:"workflow_id"`
	URL          string     `json:"url"`
	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
	RunStartedAt *time.Time `json:"run_started_at"`
}

type PipelinesFetcherGitea struct {
	*base.PipelinesFetcherBase
	*ClientGitea
}

func (p *PipelinesFetcherGitea) Run(ctx context.Context) {
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

func (p *PipelinesFetcherGitea) RunFresh(ctx context.Context) {
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

func (p *PipelinesFetcherGitea) AddPipeline(projectName string, pipeline base.PipelineInfo) error {
	pipeline_gitea, cast := pipeline.(*PipelineInfoGitea)
	if !cast {
		return errors.New("Failed to cast pipeline to gitlab pipeline")
	}
	return p.Db.AddPipeline(&models.Pipeline{
		ID:        pipeline_gitea.ID,
		Task:      base.ParseTaskFromBranch(pipeline_gitea.HeadBranch),
		Status:    pipeline_gitea.Status,
		Project:   projectName,
		StartedAt: *pipeline_gitea.CreatedAt,
	})
}

// So dirty hack FIXME(shaprunovk)
// But there is no analogue of https://try.gitea.io/api/swagger#/repository/ListActionTasks in gitea go-sdk yet
func (p *PipelinesFetcherGitea) GetPipelines(project string) ([]PipelineInfoGitea, error) {
	apiURL := fmt.Sprintf("https://gitea.com/api/v1/repos/%s/%s/actions/tasks", p.Config.Platform.Gitea.Organization.Name, project)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		p.Logger.Error("Failed to request pipelines via API", zap.Error(err))
		return nil, errors.Wrap(err, "Failed to make pipelines fetch request itself")
	}
	// Make page iteration TODO(shaprunovk)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+p.Config.Platform.Gitea.Api.Token)
	// req.SetPathValue("page", fmt.Sprint(1))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		p.Logger.Error("Failed to request pipelines via API", zap.Error(err))
		return nil, errors.Wrap(err, "Failed to request pipelines via API")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var response PipelinesInfoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		p.Logger.Error("Failed to unmarshal response", zap.Error(err))
		return nil, errors.Wrap(err, "Failed to unmarshal response")
	}

	p.Logger.Info(fmt.Sprintf("Total Workflow Runs: %d\n", response.TotalCount), lf.ProjectName(project))
	return response.WorkflowRuns, nil
}

func (p *PipelinesFetcherGitea) Fetch(id int, project string) (base.PipelineInfo, error) {
	log := p.Logger.With(
		lf.PipelineID(id),
		lf.ProjectName(project),
	)
	log.Debug("Fetching pipeline")

	pipelines, err := p.GetPipelines(project)
	if err != nil {
		log.Error("Failed to fetch pipeline", zap.Error(err))
		return nil, errors.Wrap(err, "Failed to fetch pipeline")
	}

	found := false
	var pipelineInfo PipelineInfoGitea
	for _, pipeline := range pipelines {
		if pipeline.ID == id {
			found = true
			pipelineInfo = pipeline
			break
		}
	}
	if !found {
		log.Error("Failed to fetch pipeline", zap.Error(err))
		return nil, errors.Wrap(err, fmt.Sprintf("Failed to fetch pipeline with id:%d", id))
	}
	info := &PipelineInfoGitea{
		ID:         pipelineInfo.ID,
		HeadBranch: pipelineInfo.HeadBranch,
		Status:     pipelineInfo.Status,
		CreatedAt:  pipelineInfo.CreatedAt,
	}
	return info, p.AddPipeline(project, info)
}

func (p *PipelinesFetcherGitea) FetchAllPipelines() {
	p.Logger.Debug("Start pipelines fetcher iteration")
	defer p.Logger.Debug("Finish pipelines fetcher iteration")

	err := p.ForEachProject(func(project base.Project) error {
		project_gitea, cast := project.(*gitea.Repository)
		if !cast {
			return errors.New("Failed to cast project to gitea project")
		}
		p.Logger.Debug("Found project", lf.ProjectName(project_gitea.Name))
		pipelines, err := p.GetPipelines(project_gitea.Name)
		if err != nil {
			p.Logger.Error("Failed to list projects", zap.Error(err))
			return err
		}

		for _, pipeline := range pipelines {
			p.Logger.Debug("Found pipeline", lf.ProjectName(project_gitea.Name), lf.PipelineID(pipeline.ID), lf.PipelineStatus(pipeline.Status))
			if err = p.AddPipeline(project_gitea.Name, &pipeline); err != nil {
				p.Logger.Error("Failed to add pipeline", zap.Error(err), lf.ProjectName(project_gitea.Name), lf.PipelineID(pipeline.ID))
			}
		}

		return nil
	})

	if err == nil {
		p.Logger.Debug("Successfully fetched pipelines")
	} else {
		p.Logger.Error("Failed to fetch pipelines", zap.Error(err))
	}
}

func (p *PipelinesFetcherGitea) ForEachProject(callback func(project base.Project) error) error {
	options := gitlab.ListGroupProjectsOptions{}

	for {
		opts := gitea.ListOrgReposOptions{}
		projects, resp, err := p.Gitea.ListOrgRepos(p.Config.Platform.Gitea.Organization.Name, opts)
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

		if resp.FirstPage >= resp.LastPage {
			break
		}
		options.Page = resp.NextPage
	}

	return nil
}

func (p *PipelinesFetcherGitea) FetchFreshPipelines() {
	removed := make([]interface{}, 0)
	p.Fresh.Range(func(key, _ interface{}) bool {
		id := key.(*base.QualifiedPipelineID)
		info, err := p.Fetch(id.Id, id.Project)
		info_gitea, cast := info.(*PipelineInfoGitea)
		if !cast {
			p.Logger.Error("Failed to cast pipeline info to gitea pipeline info", lf.ProjectName(id.Project), lf.PipelineID(id.Id))
		}
		if err != nil {
			p.Logger.Error("Failed to fetch pipeline", zap.Error(err))
		} else if info_gitea.Status != models.PipelineStatusRunning {
			p.Logger.Info("Fetched fresh pipeline", lf.ProjectName(id.Project), lf.PipelineID(id.Id), lf.PipelineStatus(info_gitea.Status))
			removed = append(removed, id)
		}
		return true
	})

	for _, id := range removed {
		p.Fresh.Delete(id)
	}
}
