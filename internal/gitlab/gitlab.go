package gitlab

import (
	goerrors "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
)

func Main() {
	fmt.Println("vim-go")
}

type Client struct {
	config *config.Config
	gitlab *gitlab.Client
	logger *zap.Logger
}

func NewClient(conf *config.Config, logger *zap.Logger) (*Client, error) {
	client, err := gitlab.NewClient(conf.GitLab.Api.Token, gitlab.WithBaseURL(conf.GitLab.BaseURL))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create gitlab client")
	}
	return &Client{
		config: conf,
		gitlab: client,
		logger: logger,
	}, nil
}

const (
	master = "master"
)

func (c Client) InitializeProject(user *models.User) error {
	if user.GitlabID == nil || user.GitlabLogin == nil {
		c.logger.Error("Empty gitlab user", zap.Uint("uid", user.ID))
		return errors.New("Empty gitlab user")
	}

	log := c.logger.With(zap.Stringp("gitlab_login", user.GitlabLogin), zap.Intp("gitlab_id", user.GitlabID), zap.Uint("user_id", user.ID))
	log.Info("Going to initialize project")

	projectName := c.MakeProjectName(user)
	log = log.With(lf.ProjectName(projectName))

	// Try to find existing project
	project, resp, err := c.gitlab.Projects.GetProject(fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, projectName), &gitlab.GetProjectOptions{})
	if err != nil && resp == nil {
		log.Error("Failed to get project", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, projectName)), zap.Error(err))
		return errors.Wrap(err, "Failed to get project")
	} else if resp.StatusCode == http.StatusNotFound {
		log.Info("Project was not found", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, projectName)))
		// Create project
		project, _, err = c.gitlab.Projects.CreateProject(&gitlab.CreateProjectOptions{
			Name:                 &projectName,
			NamespaceID:          &c.config.GitLab.Group.ID,
			DefaultBranch:        gitlab.String(master),
			Visibility:           gitlab.Visibility(gitlab.PrivateVisibility),
			SharedRunnersEnabled: gitlab.Bool(false),
			CIConfigPath:         &c.config.GitLab.CIConfigPath,
		})
		if err != nil {
			log.Error("Failed to create project", zap.Error(err))
			return errors.Wrap(err, "Failed to create project")
		}
		log = log.With(zap.Int("project_id", project.ID))
		log.Info("Created project")
	} else if err != nil {
		log.Error("Failed to find project", zap.Error(err))
		return errors.Wrap(err, "Failed to find project")
	} else {
		log = log.With(zap.Int("project_id", project.ID))
		log.Info("Found existing project")
	}

	// Prepare README.md with basic info
	_, _, err = c.gitlab.Commits.CreateCommit(project.ID, &gitlab.CreateCommitOptions{
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
		log.Warn("Failed to create README (file already exists)", zap.Error(err))
		// continue
	} else if err != nil {
		return errors.Wrap(err, "Failed to create README")
	}

	if err != nil {
		log.Info("Created README")
	}

	// Protect master branch from unintended commits
	_, _, err = c.gitlab.Branches.ProtectBranch(project.ID, master, &gitlab.ProtectBranchOptions{
		DevelopersCanPush:  gitlab.Bool(false),
		DevelopersCanMerge: gitlab.Bool(false),
	})
	if err != nil {
		log.Error("Failed to protect master branch", zap.Error(err))
		return errors.Wrap(err, "Failed to protect master branch")
	}
	log.Info("Protected master branch")

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
	} else {
		// Add our dear user to the project
		_, _, err = c.gitlab.ProjectMembers.AddProjectMember(project.ID, &gitlab.AddProjectMemberOptions{
			UserID:      *user.GitlabID,
			AccessLevel: gitlab.AccessLevel(gitlab.DeveloperPermissions),
		})
		if err != nil {
			log.Error("Failed to add user to the project", zap.Error(err))
			return errors.Wrap(err, "Failed to add user to the project")
		}
		log.Info("Added user to the project")
	}

	return nil
}

func cleanupName(name string) string {
	return strings.ReplaceAll(name, "-", "")
}

func (c Client) MakeProjectName(user *models.User) string {
	return fmt.Sprintf("%s-%s-%s-%s", user.GroupName, cleanupName(user.FirstName), cleanupName(user.LastName), *user.GitlabLogin)
}

func (c Client) MakeProjectURL(user *models.User) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, name)
}

func (c Client) MakeProjectSubmitsURL(user *models.User) string {
	url := c.MakeProjectURL(user)
	return fmt.Sprintf("%s/-/jobs", url)
}

func (c Client) MakeProjectWithNamespace(project string) string {
	return fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, project)
}

func (c Client) MakePipelineURL(user *models.User, pipeline *models.Pipeline) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/-/pipelines/%d", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, name, pipeline.ID)
}

func (c Client) MakeBranchURL(user *models.User, pipeline *models.Pipeline) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/-/tree/submits/%s", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, name, pipeline.Task)
}

func (c Client) MakeTaskURL(task string) string {
	return fmt.Sprintf("%s/%s", c.config.GitLab.TaskUrlPrefix, task)
}
