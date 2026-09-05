package gitlab

import (
	goerrors "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alexsergivan/transliterator"
	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
)

type Client struct {
	config   *config.Config
	gitlab   *gitlab.Client
	logger   *zap.Logger
	translit *transliterator.Transliterator
}

func NewClient(conf *config.Config, logger *zap.Logger) (*Client, error) {
	client, err := gitlab.NewClient(conf.GitLab.Api.Token, gitlab.WithBaseURL(conf.GitLab.BaseURL))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create gitlab client")
	}
	return &Client{
		config:   conf,
		gitlab:   client,
		logger:   logger,
		translit: transliterator.NewTransliterator(nil),
	}, nil
}

const (
	master = "master"
)

// ErrProjectNotReady is returned while gitlab is still forking the template
// into the student project; the caller retries later.
var ErrProjectNotReady = errors.New("project is not ready yet")

func (c Client) InitializeProject(user *models.User) (projectURL string, err error) {
	if user.GitlabID == nil || user.GitlabLogin == nil {
		c.logger.Error("Empty gitlab user", zap.Uint("uid", user.ID))
		return "", errors.New("Empty gitlab user")
	}

	log := c.logger.With(zap.Stringp("gitlab_login", user.GitlabLogin), zap.Intp("gitlab_id", user.GitlabID), zap.Uint("user_id", user.ID))
	log.Info("Going to initialize project")

	projectName := c.makeProjectName(user)
	log = log.With(lf.ProjectName(projectName))

	project, err := c.findOrCreateProject(log, projectName)
	if err != nil {
		return "", err
	}
	log = log.With(zap.Int("project_id", project.ID))

	// Fill the repository: wait for the fork of the template, or commit a
	// README into the empty project
	branch := master
	if c.useTemplateProject() {
		if err = c.checkForkReady(log, project); err != nil {
			return "", err
		}
		branch = project.DefaultBranch
	} else if err = c.createReadme(log, project); err != nil {
		return "", err
	}

	if err = c.configureProject(log, project); err != nil {
		return "", err
	}

	if err = c.protectBranch(log, project, branch); err != nil {
		return "", err
	}

	if err = c.addProjectMember(log, project, user); err != nil {
		return "", err
	}

	return c.makeProjectURL(projectName), nil
}

// useTemplateProject reports whether student repositories are forked from the
// template project (merge request courses) or created empty with README.
func (c Client) useTemplateProject() bool {
	return c.config.GitLab.TemplateProject != ""
}

func (c Client) findOrCreateProject(log *zap.Logger, projectName string) (*gitlab.Project, error) {
	escapedProject := fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, projectName)
	project, resp, err := c.gitlab.Projects.GetProject(escapedProject, &gitlab.GetProjectOptions{})
	if err == nil {
		log.Info("Found existing project", zap.Int("project_id", project.ID))
		return project, nil
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		log.Error("Failed to get project", zap.String("escaped_project", escapedProject), zap.Error(err))
		return nil, errors.Wrap(err, "Failed to get project")
	}
	log.Info("Project was not found", zap.String("escaped_project", escapedProject))

	if c.useTemplateProject() {
		// Forking is asynchronous: gitlab returns the project at once and
		// fills the repository in the background, see checkForkReady.
		// Path must be explicit: the fork API defaults it to the path of
		// the template, not to the name
		project, _, err = c.gitlab.Projects.ForkProject(c.config.GitLab.TemplateProject, &gitlab.ForkProjectOptions{
			Name:                          &projectName,
			Path:                          &projectName,
			NamespaceID:                   &c.config.GitLab.Group.ID,
			Visibility:                    gitlab.Visibility(gitlab.PrivateVisibility),
			MergeRequestDefaultTargetSelf: gitlab.Bool(true),
		})
		if err != nil {
			log.Error("Failed to fork template project", zap.String("template", c.config.GitLab.TemplateProject), zap.Error(err))
			return nil, errors.Wrap(err, "Failed to fork template project")
		}
		log.Info("Forked template project", zap.Int("project_id", project.ID))
		return project, nil
	}

	project, _, err = c.gitlab.Projects.CreateProject(&gitlab.CreateProjectOptions{
		Name:          &projectName,
		NamespaceID:   &c.config.GitLab.Group.ID,
		DefaultBranch: gitlab.String(master),
		Visibility:    gitlab.Visibility(gitlab.PrivateVisibility),
	})
	if err != nil {
		log.Error("Failed to create project", zap.Error(err))
		return nil, errors.Wrap(err, "Failed to create project")
	}
	log.Info("Created project", zap.Int("project_id", project.ID))
	return project, nil
}

// forkFinished classifies gitlab's import_status of a forked project.
func forkFinished(status string) (ready bool, failed bool) {
	switch status {
	case "finished", "none", "":
		return true, false
	case "failed":
		return false, true
	default: // scheduled, started
		return false, false
	}
}

// configureProject applies the course CI settings; a fork does not inherit
// them from the template, so this is done after creation for both kinds of
// projects.
func (c Client) configureProject(log *zap.Logger, project *gitlab.Project) error {
	_, _, err := c.gitlab.Projects.EditProject(project.ID, &gitlab.EditProjectOptions{
		SharedRunnersEnabled: gitlab.Bool(true),
		CIConfigPath:         &c.config.GitLab.CIConfigPath,
	})
	if err != nil {
		log.Error("Failed to configure project", zap.Error(err))
		return errors.Wrap(err, "Failed to configure project")
	}
	return nil
}

// checkForkReady makes sure the forked repository is complete before the
// student gets it. A failed fork is deleted so the next attempt forks again;
// a pending one is retried later by the caller.
func (c Client) checkForkReady(log *zap.Logger, project *gitlab.Project) error {
	ready, failed := forkFinished(project.ImportStatus)
	if failed {
		log.Error("Fork failed, deleting the project", zap.String("import_error", project.ImportError))
		if _, err := c.gitlab.Projects.DeleteProject(project.ID); err != nil {
			return errors.Wrap(err, "Failed to delete failed fork")
		}
		return errors.New("Fork failed, project deleted")
	}
	if !ready || project.DefaultBranch == "" {
		log.Info("Fork in progress", zap.String("import_status", project.ImportStatus))
		return ErrProjectNotReady
	}
	return nil
}

// createReadme commits the README with basic info into the empty project.
func (c Client) createReadme(log *zap.Logger, project *gitlab.Project) error {
	_, _, err := c.gitlab.Commits.CreateCommit(project.ID, &gitlab.CreateCommitOptions{
		Branch:        gitlab.String(master),
		CommitMessage: gitlab.String("Initialize repo"),
		AuthorName:    gitlab.String("notmanytask"),
		AuthorEmail:   gitlab.String("mail@notmanytask.org"),
		Actions: []*gitlab.CommitActionOptions{{
			Action:   gitlab.FileAction(gitlab.FileCreate),
			FilePath: gitlab.String("README.md"),
			Content:  gitlab.String(c.config.GitLab.DefaultReadme),
		}},
	})

	var errresp *gitlab.ErrorResponse
	// I'm sorry
	if err != nil && goerrors.As(err, &errresp) && errresp.Message == "{message: A file with this name already exists}" {
		log.Warn("Failed to create README: file already exists", zap.Error(err))
		// continue
	} else if err != nil && goerrors.As(err, &errresp) && errresp.Message == "{message: 403 Forbidden - You are not allowed to push into this branch}" {
		log.Warn("Failed to create README: main branch is protected", zap.Error(err))
		// continue
	} else if err != nil {
		return errors.Wrap(err, "Failed to create README")
	}

	if err == nil {
		log.Info("Created README")
	}
	return nil
}

// protectBranch protects the branch from unintended commits: only
// maintainers (the robot) may push and merge.
func (c Client) protectBranch(log *zap.Logger, project *gitlab.Project, branch string) error {
	log = log.With(lf.BranchName(branch))
	_, _, err := c.gitlab.ProtectedBranches.ProtectRepositoryBranches(project.ID, &gitlab.ProtectRepositoryBranchesOptions{
		Name:                 gitlab.String(branch),
		PushAccessLevel:      gitlab.AccessLevel(gitlab.MaintainerPermissions),
		MergeAccessLevel:     gitlab.AccessLevel(gitlab.MaintainerPermissions),
		UnprotectAccessLevel: gitlab.AccessLevel(gitlab.MaintainerPermissions),
	})
	var errresp *gitlab.ErrorResponse
	if err != nil {
		if goerrors.As(err, &errresp) && errresp.Message == fmt.Sprintf("{message: Protected branch '%s' already exists}", branch) {
			log.Warn("Branch is already protected", zap.Error(err))
			return nil
		}
		log.Error("Failed to protect branch", zap.Error(err))
		return errors.Wrap(err, "Failed to protect branch")
	}
	log.Info("Protected branch")
	return nil
}

func (c Client) addProjectMember(log *zap.Logger, project *gitlab.Project, user *models.User) error {
	// Check if user is alreay in project
	foundUser := false
	options := gitlab.ListProjectMembersOptions{}
	for {
		members, resp, err := c.gitlab.ProjectMembers.ListAllProjectMembers(project.ID, &options)
		if err != nil {
			log.Error("Failed to list project members", zap.Error(err))
			return errors.Wrap(err, "Failed to list project members")
		}

		for _, member := range members {
			if member.ID == *user.GitlabID {
				foundUser = true
				break
			}
		}

		if foundUser {
			break
		}

		if resp.CurrentPage >= resp.TotalPages {
			break
		}
		options.Page = resp.NextPage
	}

	if foundUser {
		log.Info("User is already in the project")
		return nil
	}

	// Add our dear user to the project
	_, _, err := c.gitlab.ProjectMembers.AddProjectMember(project.ID, &gitlab.AddProjectMemberOptions{
		UserID:      *user.GitlabID,
		AccessLevel: gitlab.AccessLevel(gitlab.DeveloperPermissions),
	})
	if err != nil {
		log.Error("Failed to add user to the project", zap.Error(err))
		return errors.Wrap(err, "Failed to add user to the project")
	}
	log.Info("Added user to the project")
	return nil
}

func (c Client) cleanupName(name string) string {
	transliteratedName := c.translit.Transliterate(name, "en")
	return strings.Map(func(ch rune) rune {
		switch ch {
		case '-':
			return -1
		case '\'':
			return -1
		}
		return ch
	}, transliteratedName)
}

func (c Client) cleanupLogin(login string) string {
	return strings.ReplaceAll(login, "__", "")
}

// makeProjectName generates a new project name for creating a repository.
// This is only used during project initialization.
func (c Client) makeProjectName(user *models.User) string {
	return fmt.Sprintf(
		"%s-%s-%s-%s",
		user.GroupName,
		c.cleanupName(user.FirstName),
		c.cleanupName(user.LastName),
		c.cleanupLogin(*user.GitlabLogin),
	)
}

func (c Client) makeProjectURL(projectName string) string {
	return fmt.Sprintf("%s/%s/%s", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, projectName)
}

func (c Client) MakeProjectURL(user *models.User) string {
	return c.makeProjectURL(user.GetProjectName())
}

func (c Client) MakeProjectSubmitsURL(user *models.User) string {
	return fmt.Sprintf("%s/-/jobs", c.MakeProjectURL(user))
}

func (c Client) MakeProjectWithNamespace(project string) string {
	return fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, project)
}

func (c Client) MakePipelineURL(user *models.User, pipeline *models.Pipeline) string {
	return fmt.Sprintf("%s/-/pipelines/%d", c.MakeProjectURL(user), pipeline.ID)
}

func (c Client) MakeBranchURL(user *models.User, pipeline *models.Pipeline) string {
	return fmt.Sprintf("%s/-/tree/submits/%s", c.MakeProjectURL(user), pipeline.Task)
}

func (c Client) MakeMergeRequestURL(user *models.User, mergeRequest *models.MergeRequest) string {
	return fmt.Sprintf("%s/-/merge_requests/%d", c.MakeProjectURL(user), mergeRequest.IID)
}

func (c Client) MakeTaskURL(task string) string {
	return fmt.Sprintf("%s/%s", c.config.GitLab.TaskUrlPrefix, task)
}

func (c Client) forEachProject(callback func(project *gitlab.Project) error) error {
	options := gitlab.ListGroupProjectsOptions{}

	for {
		projects, resp, err := c.gitlab.Groups.ListGroupProjects(c.config.GitLab.Group.ID, &options)
		if err != nil {
			c.logger.Error("Failed to list projects", zap.Error(err))
			return err
		}

		for _, project := range projects {
			if err = callback(project); err != nil {
				c.logger.Error("Project callback failed", zap.Error(err))
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
