package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	sdk "github.com/xanzy/go-gitlab"
	"go.uber.org/zap"
)

const forkRequestTimeout = 30 * time.Second

// ForksSyncer uses GitLab's Update fork operation. It never force-pushes or
// resolves conflicts, and never updates submission branches.
type ForksSyncer struct {
	*Client
	logger *zap.Logger
	http   *http.Client
	pace   time.Duration
}

func NewForksSyncer(client *Client) *ForksSyncer {
	return &ForksSyncer{
		Client: client,
		logger: client.logger.Named("forks"),
		http: &http.Client{
			Timeout: forkRequestTimeout,
			// Do not forward the API token through a redirect.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		pace: time.Second,
	}
}

func (s *ForksSyncer) Run(ctx context.Context) {
	interval := s.config.PullIntervals.Forks
	if interval == nil || *interval <= 0 || s.config.GitLab.TemplateProject == "" {
		return
	}
	for ctx.Err() == nil {
		if err := s.iteration(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("Failed to synchronize course forks; retrying next sweep", zap.Error(err))
		}
		// No overlapping sweeps or catch-up bursts after a slow GitLab response.
		if !waitForForkSync(ctx, *interval) {
			return
		}
	}
}

func waitForForkSync(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (s *ForksSyncer) iteration(ctx context.Context) error {
	requestCtx, cancel := context.WithTimeout(ctx, forkRequestTimeout)
	template, _, err := s.gitlab.Projects.GetProject(s.config.GitLab.TemplateProject, nil, sdk.WithContext(requestCtx))
	cancel()
	if err != nil {
		return fmt.Errorf("resolve template: %w", err)
	}
	if template.ID == 0 || template.DefaultBranch == "" || template.Archived {
		return fmt.Errorf("template is missing its default branch or is archived")
	}
	branch := template.DefaultBranch
	if mr := s.config.GitLab.MergeRequests; mr != nil {
		branch = mr.GetTargetBranch()
	}

	options := &sdk.ListGroupProjectsOptions{
		ListOptions:      sdk.ListOptions{PerPage: 100, Page: 1},
		Simple:           sdk.Bool(true),
		WithShared:       sdk.Bool(false),
		IncludeSubGroups: sdk.Bool(false),
		Archived:         sdk.Bool(false),
		OrderBy:          sdk.String("id"),
		Sort:             sdk.String("asc"),
	}
	checked, requested, failed := 0, 0, 0
	for {
		requestCtx, cancel := context.WithTimeout(ctx, forkRequestTimeout)
		projects, response, err := s.gitlab.Groups.ListGroupProjects(s.config.GitLab.Group.ID, options, sdk.WithContext(requestCtx))
		cancel()
		if err != nil {
			return fmt.Errorf("list course projects: %w", err)
		}
		for _, project := range projects {
			// Also enforce ownership locally; shared and subgroup projects are
			// not course repositories even if an API filter is ignored.
			if project.ID == template.ID || project.Archived || project.Namespace == nil ||
				project.Namespace.ID != s.config.GitLab.Group.ID || project.PathWithNamespace == "" {
				continue
			}
			if !waitForForkSync(ctx, s.pace) {
				return ctx.Err()
			}
			checked++
			started, err := s.syncProject(ctx, project.PathWithNamespace, template.ID, branch)
			if err != nil {
				failed++
				s.logger.Warn("Fork synchronization failed; retrying next sweep",
					zap.String("project", project.PathWithNamespace), zap.Error(err))
			} else if started {
				requested++
			}
		}
		if response.NextPage == 0 {
			break
		}
		if response.NextPage <= options.Page {
			return fmt.Errorf("non-advancing course project pagination")
		}
		options.Page = response.NextPage
	}
	s.logger.Info("Finished fork synchronization sweep", zap.Int("checked", checked),
		zap.Int("requested", requested), zap.Int("failed", failed))
	return ctx.Err()
}

// Nullable GraphQL fields must not silently turn into false/zero: an unknown
// state is not permission to start a repository mutation.
type forkDetails struct {
	Behind       *int  `json:"behind"`
	HasConflicts *bool `json:"hasConflicts"`
	IsSyncing    *bool `json:"isSyncing"`
}

const forkStatusQuery = `query ForkSyncStatus($path: ID!, $branch: String!) {
  project(fullPath: $path) {
    forkedFrom { id }
    forkDetails(ref: $branch) { behind hasConflicts isSyncing }
  }
}`

const syncForkMutation = `mutation SyncCourseFork($path: ID!, $branch: String!) {
  projectSyncFork(input: {projectPath: $path, targetBranch: $branch}) {
    errors
  }
}`

func (s *ForksSyncer) syncProject(ctx context.Context, path string, templateID int, branch string) (bool, error) {
	variables := map[string]string{"path": path, "branch": branch}
	var status struct {
		Project *struct {
			ForkedFrom *struct{ ID string } `json:"forkedFrom"`
			Details    *forkDetails         `json:"forkDetails"`
		} `json:"project"`
	}
	if err := s.graphql(ctx, forkStatusQuery, variables, &status); err != nil {
		return false, err
	}
	if status.Project == nil {
		return false, fmt.Errorf("fork is unavailable")
	}
	project := status.Project
	if project.ForkedFrom == nil || project.ForkedFrom.ID != fmt.Sprintf("gid://gitlab/Project/%d", templateID) {
		return false, nil
	}
	details := project.Details
	if details == nil || details.Behind == nil || details.HasConflicts == nil || details.IsSyncing == nil {
		return false, fmt.Errorf("fork comparison is unavailable")
	}
	if *details.Behind <= 0 || *details.IsSyncing {
		return false, nil
	}
	if *details.HasConflicts {
		s.logger.Warn("Fork has conflicts; manual synchronization required", zap.String("project", path), zap.String("branch", branch))
		return false, nil
	}
	if !waitForForkSync(ctx, s.pace) {
		return false, ctx.Err()
	}
	var result struct {
		Sync *struct {
			Errors []string `json:"errors"`
		} `json:"projectSyncFork"`
	}
	if err := s.graphql(ctx, syncForkMutation, variables, &result); err != nil {
		return false, err
	}
	if result.Sync == nil {
		return false, fmt.Errorf("missing fork synchronization result")
	}
	if len(result.Sync.Errors) > 0 {
		return false, fmt.Errorf("fork synchronization: %v", result.Sync.Errors)
	}
	// GitLab enqueues a background job. An accepted request is not proof
	// of completion: the next sweep checks behind/isSyncing/conflicts again.
	s.logger.Info("Requested fork synchronization", zap.String("project", path), zap.String("branch", branch))
	return true, nil
}

func (s *ForksSyncer) graphql(ctx context.Context, query string, variables map[string]string, result any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	// BaseURL ends in /api/v4/; preserve an optional GitLab installation prefix.
	endpoint := s.gitlab.BaseURL().ResolveReference(&url.URL{Path: "../graphql"})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("PRIVATE-TOKEN", s.config.GitLab.Api.Token)
	response, err := s.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitLab GraphQL HTTP %d", response.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("decode GraphQL response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("GitLab GraphQL: %v", envelope.Errors)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("missing GraphQL data")
	}
	return json.Unmarshal(envelope.Data, result)
}
