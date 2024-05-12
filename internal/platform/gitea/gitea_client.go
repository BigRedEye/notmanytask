package gitea

import (
	"fmt"
	"net/http"

	"code.gitea.io/sdk/gitea"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/platform/base"
)

type ClientGitea struct {
	*base.ClientBase
	Gitea *gitea.Client
}

func (c *ClientGitea) InitializeProject(user *models.User) error {
	if user.GiteaID == nil || user.GiteaLogin == nil {
		c.Logger.Error("Empty gitea user", zap.Uint("uid", user.ID))
		return errors.New("Empty gitea user")
	}

	log := c.Logger.With(zap.Stringp("gitea_login", user.GiteaLogin), zap.Int64p("gitea_id", user.GiteaID), zap.Uint("user_id", user.ID))
	log.Info("Going to initialize project")

	projectName := c.MakeProjectName(user)
	log = log.With(lf.ProjectName(projectName))

	// Try to find existing repository
	repo, resp, err := c.Gitea.GetRepo(c.Config.Platform.Gitea.Organization.Name, projectName)
	if err != nil && resp == nil {
		log.Error("Failed to get repository", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.Config.Platform.Gitea.Organization.Name, projectName)), zap.Error(err))
		return errors.Wrap(err, "Failed to get repository")
	} else if resp.StatusCode == http.StatusNotFound {
		log.Info("Repository was not found", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.Config.Platform.Gitea.Organization.Name, projectName)))
		// Create repository

		opts := gitea.CreateRepoOption{
			Name:          projectName,
			Description:   c.Config.Platform.Gitea.DefaultReadme,
			DefaultBranch: base.Master,
			Private:       true,
			AutoInit:      true,
			Template:      true,
		}
		repo, _, err = c.Gitea.CreateOrgRepo(c.Config.Platform.Gitea.Organization.Name, opts)
		if err != nil {
			log.Error("Failed to create repository", zap.Error(err))
			return errors.Wrap(err, "Failed to create repository")
		}
		log = log.With(zap.Int64("repository_id", repo.ID))
		log.Info("Created repository")
	} else if err != nil {
		log.Error("Failed to find repository", zap.Error(err))
		return errors.Wrap(err, "Failed to find repository")
	} else {
		log = log.With(zap.Int64("repository_id", repo.ID))
		log.Info("Found existing repository")
	}

	// Protect master branch from unintended commits

	branch_protection_opts := gitea.CreateBranchProtectionOption{
		BranchName:           base.Master,
		EnablePush:           false,
		EnableMergeWhitelist: false,
	}
	_, _, err = c.Gitea.CreateBranchProtection(c.Config.Platform.Gitea.Organization.Name, repo.Name, branch_protection_opts)
	if err != nil {
		log.Error("Failed to protect master branch", zap.Error(err))
		return errors.Wrap(err, "Failed to protect master branch")
	}
	log.Info("Protected master branch")

	// Check if user is already in project
	foundUser := false
	options := gitea.ListCollaboratorsOptions{}
	for {
		members, resp, err := c.Gitea.ListCollaborators(c.Config.Platform.Gitea.Organization.Name, repo.Name, options)
		if err != nil {
			log.Error("Failed to list repository members", zap.Error(err))
			return errors.Wrap(err, "Failed to list repository members")
		}

		for _, member := range members {
			if member.ID == *user.GiteaID {
				foundUser = true
				break
			}
		}

		if foundUser {
			break
		}

		if resp.FirstPage >= resp.LastPage {
			break
		}
		options.Page = resp.NextPage
	}

	if foundUser {
		log.Info("User is already in the repository")
	} else {
		// Add our dear user to the project
		permission := gitea.AccessModeWrite
		col_opts := gitea.AddCollaboratorOption{
			Permission: &permission,
		}
		_, err = c.Gitea.AddCollaborator(c.Config.Platform.Gitea.Organization.Name, repo.Name, *user.GiteaLogin, col_opts)
		if err != nil {
			log.Error("Failed to add user to the repository", zap.Error(err))
			return errors.Wrap(err, "Failed to add user to the repository")
		}
		log.Info("Added user to the repository")
	}

	return nil
}

func (c *ClientGitea) MakeProjectName(user *models.User) string {
	return fmt.Sprintf("%s-%s-%s-%s", user.GroupName, c.CleanupName(user.FirstName), c.CleanupName(user.LastName), c.CleanupLogin(*user.GiteaLogin))
}

func (c *ClientGitea) MakeProjectURL(user *models.User) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s", c.Config.Platform.Gitea.BaseURL, c.Config.Platform.Gitea.Organization.Name, name)
}

func (c *ClientGitea) MakeProjectSubmitsURL(user *models.User) string {
	url := c.MakeProjectURL(user)
	return fmt.Sprintf("%s/actions", url)
}

func (c *ClientGitea) MakeProjectWithNamespace(project string) string {
	return fmt.Sprintf("%s/%s", c.Config.Platform.Gitea.Organization.Name, project)
}

func (c *ClientGitea) MakePipelineURL(user *models.User, pipeline *models.Pipeline) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/actions/runs/%d", c.Config.Platform.Gitea.BaseURL, c.Config.Platform.Gitea.Organization.Name, name, pipeline.ID)
}

func (c *ClientGitea) MakeBranchURL(user *models.User, pipeline *models.Pipeline) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/src/branch/%s", c.Config.Platform.Gitea.BaseURL, c.Config.Platform.Gitea.Organization.Name, name, pipeline.Task)
}

func (c *ClientGitea) MakeTaskURL(task string) string {
	return fmt.Sprintf("%s/%s", c.Config.Platform.Gitea.TaskUrlPrefix, task)
}

var _ base.ClientInterface = &ClientGitea{}
