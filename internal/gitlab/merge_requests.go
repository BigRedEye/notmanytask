package gitlab

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
)

// MergeRequestsFetcher implements the merge-request workflow: every
// submits/<task> branch gets a merge request into the target branch, merge
// requests are synced into the database, and quiet mergeable ones are merged
// automatically after the review period.
type MergeRequestsFetcher struct {
	*Client

	logger *zap.Logger
	db     *database.DataBase
	conf   *config.MergeRequestsConfig
}

func NewMergeRequestsFetcher(client *Client, db *database.DataBase) (*MergeRequestsFetcher, error) {
	if client.config.GitLab.MergeRequests == nil {
		return nil, errors.New("merge requests are not configured")
	}
	return &MergeRequestsFetcher{
		Client: client,
		logger: client.logger.Named("merge_requests"),
		db:     db,
		conf:   client.config.GitLab.MergeRequests,
	}, nil
}

func (p *MergeRequestsFetcher) Run(ctx context.Context) {
	interval := p.config.PullIntervals.MergeRequests
	if interval == nil {
		return
	}

	tick := time.NewTicker(*interval)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			p.iteration()
		case <-ctx.Done():
			p.logger.Info("Stopping merge requests fetcher")
			return
		}
	}
}

func (p *MergeRequestsFetcher) iteration() {
	p.logger.Debug("Start merge requests iteration")
	defer p.logger.Debug("Finish merge requests iteration")

	err := p.forEachProject(func(project *gitlab.Project) error {
		log := p.logger.With(lf.ProjectName(project.Name))
		if err := p.syncProject(log, project); err != nil {
			log.Error("Failed to sync merge requests", zap.Error(err))
			return nil
		}
		if err := p.updateProject(log, project); err != nil {
			log.Error("Failed to update merge requests", zap.Error(err))
		}
		return nil
	})
	if err != nil {
		p.logger.Error("Failed to iterate projects", zap.Error(err))
	}
}

////////////////////////////////////////////////////////////////////////////////
// Sync: gitlab -> database

func (p *MergeRequestsFetcher) syncProject(log *zap.Logger, project *gitlab.Project) error {
	options := &gitlab.ListProjectMergeRequestsOptions{
		TargetBranch:           gitlab.String(p.conf.GetTargetBranch()),
		WithMergeStatusRecheck: gitlab.Bool(true),
	}
	for {
		mergeRequests, resp, err := p.gitlab.MergeRequests.ListProjectMergeRequests(project.ID, options)
		if err != nil {
			return errors.Wrap(err, "Failed to list merge requests")
		}

		for _, mr := range mergeRequests {
			if !IsSubmitBranch(mr.SourceBranch) {
				continue
			}
			// Merged requests are synced too: a re-run pipeline on the merged
			// head can take the credit away (see the scorer)
			task := ParseTaskFromBranch(mr.SourceBranch)
			if err = p.syncMergeRequest(log.With(lf.BranchName(mr.SourceBranch)), project, mr, task); err != nil {
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

func (p *MergeRequestsFetcher) syncMergeRequest(log *zap.Logger, project *gitlab.Project, mr *gitlab.MergeRequest, task string) error {
	notes, err := p.getNotesInfo(project, mr)
	if err != nil {
		return errors.Wrap(err, "Failed to list notes")
	}
	extraChanges, err := p.hasExtraChanges(project, mr, task)
	if err != nil {
		return errors.Wrap(err, "Failed to list changes")
	}
	pipeline, err := p.getLatestPipeline(project, mr)
	if err != nil {
		return errors.Wrap(err, "Failed to list pipelines")
	}

	mergeUserLogin := ""
	if mr.MergedBy != nil {
		mergeUserLogin = mr.MergedBy.Username
	}
	startedAt := time.Time{}
	if mr.CreatedAt != nil {
		startedAt = *mr.CreatedAt
	}

	err = p.db.UpsertMergeRequest(&models.MergeRequest{
		ID:                    mr.ID,
		IID:                   mr.IID,
		SHA:                   mr.SHA,
		Project:               project.Name,
		Task:                  task,
		State:                 mr.State,
		UserNotesCount:        mr.UserNotesCount,
		StartedAt:             startedAt,
		MergeStatus:           normalizeMergeStatus(mr.DetailedMergeStatus, mr.MergeStatus),
		MergeUserLogin:        mergeUserLogin,
		HasUnresolvedNotes:    notes.HasUnresolvedNotes,
		LastNoteCreatedAt:     notes.LastNoteCreatedAt,
		LastPipelineStatus:    pipeline.Status,
		LastPipelineCreatedAt: pipeline.CreatedAt,
		ExtraChanges:          extraChanges,
	})
	if err != nil {
		return errors.Wrap(err, "Failed to upsert merge request")
	}
	log.Debug("Synced merge request", lf.MergeRequestID(mr.ID), lf.MergeRequestState(mr.State))
	return nil
}

// normalizeMergeStatus maps gitlab's detailed_merge_status (15.6+) onto the
// legacy merge_status values the rest of the code understands, and falls
// back to the legacy field on older gitlab. Anything but can_be_merged
// blocks auto-merge.
func normalizeMergeStatus(detailed, legacy string) string {
	switch detailed {
	case "":
		return legacy
	case "mergeable":
		return models.MergeRequestStatusCanBeMerged
	case "conflict", "need_rebase":
		return models.MergeRequestStatusCannotBeMerged
	default:
		return detailed
	}
}

type notesInfo struct {
	HasUnresolvedNotes bool
	LastNoteCreatedAt  time.Time
}

// getNotesInfo looks at resolvable (review) notes only. Notes are listed
// newest first, so the first resolvable note is the last created one.
func (p *MergeRequestsFetcher) getNotesInfo(project *gitlab.Project, mr *gitlab.MergeRequest) (notesInfo, error) {
	options := &gitlab.ListMergeRequestNotesOptions{
		OrderBy: gitlab.String("created_at"),
		Sort:    gitlab.String("desc"),
	}

	result := notesInfo{}
	for {
		notes, resp, err := p.gitlab.Notes.ListMergeRequestNotes(project.ID, mr.IID, options)
		if err != nil {
			return result, err
		}

		for _, note := range notes {
			if !note.Resolvable {
				continue
			}
			if result.LastNoteCreatedAt.IsZero() && note.CreatedAt != nil {
				result.LastNoteCreatedAt = *note.CreatedAt
			}
			if !note.Resolved {
				result.HasUnresolvedNotes = true
				return result, nil
			}
		}

		if resp.CurrentPage >= resp.TotalPages {
			break
		}
		options.Page = resp.NextPage
	}
	return result, nil
}

// hasExtraChanges reports whether the merge request touches anything outside
// of its task directory.
func (p *MergeRequestsFetcher) hasExtraChanges(project *gitlab.Project, mr *gitlab.MergeRequest, task string) (bool, error) {
	allowedPrefix := fmt.Sprintf("%s%s/", tasksPrefix, task)

	changes, _, err := p.gitlab.MergeRequests.GetMergeRequestChanges(project.ID, mr.IID, &gitlab.GetMergeRequestChangesOptions{})
	if err != nil {
		return false, err
	}
	if changes.Overflow {
		// The diff is truncated, we cannot prove it is clean
		return true, nil
	}

	for _, change := range changes.Changes {
		if !strings.HasPrefix(change.NewPath, allowedPrefix) || !strings.HasPrefix(change.OldPath, allowedPrefix) {
			return true, nil
		}
	}
	return false, nil
}

type pipelineInfo struct {
	Status    models.PipelineStatus
	CreatedAt time.Time
}

// getLatestPipeline returns the newest pipeline of the merge request; gitlab
// lists them newest first.
func (p *MergeRequestsFetcher) getLatestPipeline(project *gitlab.Project, mr *gitlab.MergeRequest) (pipelineInfo, error) {
	pipelines, _, err := p.gitlab.MergeRequests.ListMergeRequestPipelines(project.ID, mr.IID)
	if err != nil {
		return pipelineInfo{}, err
	}

	for _, pipeline := range pipelines {
		if pipeline.CreatedAt != nil {
			return pipelineInfo{Status: pipeline.Status, CreatedAt: *pipeline.CreatedAt}, nil
		}
	}
	return pipelineInfo{}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Update: database -> gitlab (create missing merge requests, merge quiet ones)

func (p *MergeRequestsFetcher) updateProject(log *zap.Logger, project *gitlab.Project) error {
	reviewDeadline := time.Now().Add(-p.conf.ReviewTtl)

	options := &gitlab.ListBranchesOptions{}
	for {
		branches, resp, err := p.gitlab.Branches.ListBranches(project.ID, options)
		if err != nil {
			return errors.Wrap(err, "Failed to list branches")
		}

		for _, branch := range branches {
			if !IsSubmitBranch(branch.Name) {
				continue
			}
			if err := p.updateBranch(log.With(lf.BranchName(branch.Name)), project, branch, reviewDeadline); err != nil {
				log.Error("Failed to update branch merge requests", zap.Error(err), lf.BranchName(branch.Name))
			}
		}

		if resp.CurrentPage >= resp.TotalPages {
			break
		}
		options.Page = resp.NextPage
	}
	return nil
}

type branchMergeRequests struct {
	Open               *models.MergeRequest
	Merged             *models.MergeRequest
	HasUnresolvedNotes bool
	LastNoteCreatedAt  time.Time
}

func (p *MergeRequestsFetcher) updateBranch(log *zap.Logger, project *gitlab.Project, branch *gitlab.Branch, reviewDeadline time.Time) error {
	mrs, err := p.loadBranchMergeRequests(project, branch.Name)
	if err != nil {
		return err
	}

	head := ""
	if branch.Commit != nil {
		head = branch.Commit.ID
	}

	if mrs.Open == nil {
		if !needsMergeRequest(mrs, head) {
			return nil
		}
		log.Info("No merge request for the branch head, creating a new one")
		_, _, err = p.gitlab.MergeRequests.CreateMergeRequest(project.ID, &gitlab.CreateMergeRequestOptions{
			SourceBranch: gitlab.String(branch.Name),
			TargetBranch: gitlab.String(p.conf.GetTargetBranch()),
			Title:        gitlab.String(branch.Name),
		})
		if err != nil {
			return errors.Wrap(err, "Failed to create merge request")
		}
		log.Info("Created merge request")
		return nil
	}

	if p.conf.ReviewTtl == 0 || !readyToMerge(mrs, reviewDeadline) {
		return nil
	}

	log = log.With(lf.MergeRequestID(mrs.Open.ID))
	log.Info("Accepting merge request")
	// SHA makes gitlab refuse the merge if the branch moved since the sync
	_, _, err = p.gitlab.MergeRequests.AcceptMergeRequest(project.ID, mrs.Open.IID, &gitlab.AcceptMergeRequestOptions{
		MergeCommitMessage: gitlab.String("Automatic merge"),
		SHA:                gitlab.String(mrs.Open.SHA),
	})
	if err != nil {
		return errors.Wrap(err, "Failed to accept merge request")
	}
	log.Info("Accepted merge request")
	return nil
}

// needsMergeRequest reports whether a branch without an open merge request
// needs one: always for a never-merged task, and for a merged task only when
// the branch moved past the merged head (a fix after a re-check).
func needsMergeRequest(mrs *branchMergeRequests, head string) bool {
	if mrs.Merged == nil {
		return true
	}
	return head != "" && mrs.Merged.SHA != "" && head != mrs.Merged.SHA
}

// readyToMerge is the auto-merge rule: gitlab says the request is mergeable,
// the pipeline is green, nothing is unresolved, only task files are touched,
// and both the last pipeline and the last review note are older than the
// review period. Anything unknown blocks the merge until the next sync.
func readyToMerge(mrs *branchMergeRequests, reviewDeadline time.Time) bool {
	open := mrs.Open
	return open.MergeStatus == models.MergeRequestStatusCanBeMerged &&
		open.SHA != "" &&
		!mrs.HasUnresolvedNotes &&
		mrs.LastNoteCreatedAt.Before(reviewDeadline) &&
		!open.ExtraChanges &&
		open.LastPipelineStatus == models.PipelineStatusSuccess &&
		!open.LastPipelineCreatedAt.IsZero() &&
		open.LastPipelineCreatedAt.Before(reviewDeadline)
}

func (p *MergeRequestsFetcher) loadBranchMergeRequests(project *gitlab.Project, branch string) (*branchMergeRequests, error) {
	mergeRequests, err := p.db.ListProjectTaskMergeRequests(project.Name, ParseTaskFromBranch(branch))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list branch merge requests")
	}

	result := &branchMergeRequests{}
	for i := range mergeRequests {
		mr := &mergeRequests[i]
		switch mr.State {
		case models.MergeRequestStateOpened:
			if result.Open == nil || result.Open.StartedAt.Before(mr.StartedAt) {
				result.Open = mr
			}
		case models.MergeRequestStateMerged:
			if result.Merged == nil || result.Merged.StartedAt.Before(mr.StartedAt) {
				result.Merged = mr
			}
		}
		if mr.HasUnresolvedNotes {
			result.HasUnresolvedNotes = true
		}
		if mr.LastNoteCreatedAt.After(result.LastNoteCreatedAt) {
			result.LastNoteCreatedAt = mr.LastNoteCreatedAt
		}
	}
	return result, nil
}
