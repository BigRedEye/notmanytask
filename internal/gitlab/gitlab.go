package gitlab

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
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

func NewClient(config *config.Config, logger *zap.Logger) (*Client, error) {
	client, err := gitlab.NewClient(config.GitLab.Api.Token, gitlab.WithBaseURL(config.GitLab.BaseURL))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create gitlab client")
	}
	return &Client{
		config: config,
		gitlab: client,
		logger: logger,
	}, nil
}

const (
	master = "master"
)

func (c Client) InitializeProject(user *models.User) error {
	log := c.logger.With(zap.String("user_login", user.Login), zap.Int("user_id", user.ID))
	log.Info("Going to initialize project")

	projectName := c.MakeProjectName(user)
	project, _, err := c.gitlab.Projects.CreateProject(&gitlab.CreateProjectOptions{
		Name:                 &projectName,
		NamespaceID:          &c.config.GitLab.Group.ID,
		DefaultBranch:        gitlab.String(master),
		Visibility:           gitlab.Visibility(gitlab.PrivateVisibility),
		SharedRunnersEnabled: gitlab.Bool(false),
	})
	if err != nil {
		log.Error("Failed to create project", zap.Error(err), zap.String("project_name", projectName))
		return errors.Wrap(err, "Failed to create project")
	}
	log = log.With(zap.String("project_name", project.Name), zap.Int("project_id", project.ID))
	log.Info("Created project")

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
	if err != nil {
		log.Error("Failed to create README", zap.Error(err))
		return errors.Wrap(err, "Failed to create README")
	}
	log.Info("Created README")

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

	// Add our dear user to the project
	_, _, err = c.gitlab.ProjectMembers.AddProjectMember(project.ID, &gitlab.AddProjectMemberOptions{
		UserID:      user.ID,
		AccessLevel: gitlab.AccessLevel(gitlab.DeveloperPermissions),
	})
	if err != nil {
		log.Error("Failed to add user to the project", zap.Error(err))
		return errors.Wrap(err, "Failed to add user to the project")
	}
	log.Info("Added user to the project")

	return nil
}

func (c Client) MakeProjectName(user *models.User) string {
	return fmt.Sprintf("%s-%s-%s-%s", user.GroupName, user.FirstName, user.LastName, user.Login)
}

func (c Client) MakeProjectUrl(user *models.User) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, name)
}
