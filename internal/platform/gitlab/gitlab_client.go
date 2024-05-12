package gitlab

import (
	goerrors "errors"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"

	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
	base "github.com/bigredeye/notmanytask/internal/platform/base"
)

type ClientGitlab struct {
	*base.ClientBase
	Gitlab *gitlab.Client
}

func (c *ClientGitlab) InitializeProject(user *models.User) error {
	if user.GitlabID == nil || user.GitlabLogin == nil {
		c.Logger.Error("Empty gitlab user", zap.Uint("uid", user.ID))
		return errors.New("Empty gitlab user")
	}

	log := c.Logger.With(zap.Stringp("gitlab_login", user.GitlabLogin), zap.Intp("gitlab_id", user.GitlabID), zap.Uint("user_id", user.ID))
	log.Info("Going to initialize project")

	projectName := c.MakeProjectName(user)
	log = log.With(lf.ProjectName(projectName))

	// Try to find existing project
	project, resp, err := c.Gitlab.Projects.GetProject(fmt.Sprintf("%s/%s", c.Config.Platform.GitLab.Group.Name, projectName), &gitlab.GetProjectOptions{})
	if err != nil && resp == nil {
		log.Error("Failed to get project", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.Config.Platform.GitLab.Group.Name, projectName)), zap.Error(err))
		return errors.Wrap(err, "Failed to get project")
	} else if resp.StatusCode == http.StatusNotFound {
		log.Info("Project was not found", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.Config.Platform.GitLab.Group.Name, projectName)))
		// Create project
		project, _, err = c.Gitlab.Projects.CreateProject(&gitlab.CreateProjectOptions{
			Name:                 &projectName,
			NamespaceID:          &c.Config.Platform.GitLab.Group.ID,
			DefaultBranch:        gitlab.String(base.Master),
			Visibility:           gitlab.Visibility(gitlab.PrivateVisibility),
			SharedRunnersEnabled: gitlab.Bool(false),
			CIConfigPath:         &c.Config.Platform.GitLab.CIConfigPath,
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
	_, _, err = c.Gitlab.Commits.CreateCommit(project.ID, &gitlab.CreateCommitOptions{
		Branch:        gitlab.String(base.Master),
		CommitMessage: gitlab.String("Initialize repo"),
		AuthorName:    gitlab.String("notmanytask"),
		AuthorEmail:   gitlab.String("mail@notmanytask.org"),
		Actions: []*gitlab.CommitActionOptions{{
			Action:   gitlab.FileAction(gitlab.FileCreate),
			FilePath: gitlab.String("README.md"),
			Content:  gitlab.String(c.Config.Platform.DefaultReadme),
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

	// Protect base.Master branch from unintended commits
	_, _, err = c.Gitlab.Branches.ProtectBranch(project.ID, base.Master, &gitlab.ProtectBranchOptions{
		DevelopersCanPush:  gitlab.Bool(false),
		DevelopersCanMerge: gitlab.Bool(false),
	})
	if err != nil {
		log.Error("Failed to protect base.Master branch", zap.Error(err))
		return errors.Wrap(err, "Failed to protect base.Master branch")
	}
	log.Info("Protected base.Master branch")

	// Check if user is alreay in project
	foundUser := false
	options := gitlab.ListProjectMembersOptions{}
	for {
		members, resp, err := c.Gitlab.ProjectMembers.ListAllProjectMembers(project.ID, &options)
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
		_, _, err = c.Gitlab.ProjectMembers.AddProjectMember(project.ID, &gitlab.AddProjectMemberOptions{
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

func (c *ClientGitlab) MakeProjectName(user *models.User) string {
	return fmt.Sprintf("%s-%s-%s-%s", user.GroupName, c.CleanupName(user.FirstName), c.CleanupName(user.LastName), c.CleanupLogin(*user.GitlabLogin))
}

func (c *ClientGitlab) MakeProjectURL(user *models.User) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s", c.Config.Platform.GitLab.BaseURL, c.Config.Platform.GitLab.Group.Name, name)
}

func (c *ClientGitlab) MakeProjectSubmitsURL(user *models.User) string {
	url := c.MakeProjectURL(user)
	return fmt.Sprintf("%s/-/jobs", url)
}

func (c *ClientGitlab) MakeProjectWithNamespace(project string) string {
	return fmt.Sprintf("%s/%s", c.Config.Platform.GitLab.Group.Name, project)
}

func (c *ClientGitlab) MakePipelineURL(user *models.User, pipeline *models.Pipeline) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/-/pipelines/%d", c.Config.Platform.GitLab.BaseURL, c.Config.Platform.GitLab.Group.Name, name, pipeline.ID)
}

func (c *ClientGitlab) MakeBranchURL(user *models.User, pipeline *models.Pipeline) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/-/tree/submits/%s", c.Config.Platform.GitLab.BaseURL, c.Config.Platform.GitLab.Group.Name, name, pipeline.Task)
}

func (c *ClientGitlab) MakeTaskURL(task string) string {
	return fmt.Sprintf("%s/%s", c.Config.Platform.TaskUrlPrefix, task)
}

var _ base.ClientInterface = &ClientGitlab{}
