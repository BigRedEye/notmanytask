package gitlab

import (
	"fmt"
	"github.com/alexsergivan/transliterator"
	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"
	"go.uber.org/zap"
	"net/http"
	"strings"

	"github.com/bigredeye/notmanytask/internal/config"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
)

func Main() {
	fmt.Println("vim-go")
}

type Client struct {
	config   *config.Config
	gitlab   *gitlab.Client
	logger   *zap.Logger
	translit *transliterator.Transliterator
}

func NewClient(config *config.Config, logger *zap.Logger) (*Client, error) {
	client, err := gitlab.NewClient(config.GitLab.Api.Token, gitlab.WithBaseURL(config.GitLab.BaseURL))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create gitlab client")
	}
	return &Client{
		config:   config,
		gitlab:   client,
		logger:   logger,
		translit: transliterator.NewTransliterator(nil),
	}, nil
}

const (
	master = "main"
)

func (c Client) InitializeProject(user *models.User) (string, error) {
	if user.GitlabID == nil || user.GitlabLogin == nil {
		c.logger.Error("Empty gitlab user", zap.Uint("uid", user.ID))
		return "", errors.New("Empty gitlab user")
	}

	log := c.logger.With(zap.Stringp("gitlab_login", user.GitlabLogin), zap.Intp("gitlab_id", user.GitlabID), zap.Uint("user_id", user.ID))
	log.Info("Going to initialize project")

	projectName := c.BuildProjectName(user)
	log = log.With(lf.ProjectName(projectName))

	// Try to find existing project
	project, resp, err := c.gitlab.Projects.GetProject(fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, projectName), &gitlab.GetProjectOptions{})
	if err != nil && resp == nil {
		log.Error("Failed to get project", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, projectName)), zap.Error(err))
		return "", errors.Wrap(err, "Failed to get project")
	} else if resp.StatusCode == http.StatusNotFound {
		log.Info("Project was not found", zap.String("escaped_project", fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, projectName)))
		// Create project
		project, _, err = c.gitlab.Projects.CreateProject(&gitlab.CreateProjectOptions{
			Name:                 &projectName,
			NamespaceID:          &c.config.GitLab.Group.ID,
			DefaultBranch:        gitlab.String(master),
			Visibility:           gitlab.Visibility(gitlab.PrivateVisibility),
			SharedRunnersEnabled: gitlab.Bool(true),
			CIConfigPath:         &c.config.GitLab.CIConfigPath,
			ImportURL:            gitlab.String(c.config.GitLab.ProjectTemplateURL),
		})
		if err != nil {
			log.Error("Failed to create project", zap.Error(err))
			return "", errors.Wrap(err, "Failed to create project")
		}
		log = log.With(zap.Int("project_id", project.ID))
		log.Info("Created project")
	} else if err != nil {
		log.Error("Failed to find project", zap.Error(err))
		return "", errors.Wrap(err, "Failed to find project")
	} else {
		log = log.With(zap.Int("project_id", project.ID))
		log.Info("Found existing project")
	}

	// Check if user is alreay in project
	foundUser := false
	options := gitlab.ListProjectMembersOptions{}
	for {
		members, resp, err := c.gitlab.ProjectMembers.ListAllProjectMembers(project.ID, &options)
		if err != nil {
			log.Error("Failed to list project members", zap.Error(err))
			return "", errors.Wrap(err, "Failed to list project members")
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
			return "", errors.Wrap(err, "Failed to add user to the project")
		}
		log.Info("Added user to the project")
	}

	return projectName, nil
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

func (c Client) BuildProjectName(user *models.User) string {
	return fmt.Sprintf("%s-%s-%s-%s", user.GroupName, c.cleanupName(user.FirstName), c.cleanupName(user.LastName), c.cleanupLogin(*user.GitlabLogin))
}

func (c Client) MakeProjectName(user *models.User) string {
	if user.ProjectName == nil {
		return ""
	}
	return *user.ProjectName
}

func (c Client) MakeProjectUrl(user *models.User) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, name)
}

func (c Client) MakeProjectSubmitsUrl(user *models.User) string {
	url := c.MakeProjectUrl(user)
	return fmt.Sprintf("%s/-/jobs", url)
}

func (c Client) MakeProjectWithNamespace(project string) string {
	return fmt.Sprintf("%s/%s", c.config.GitLab.Group.Name, project)
}

func (c Client) MakePipelineUrl(user *models.User, pipeline *models.Pipeline) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/-/pipelines/%d", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, name, pipeline.ID)
}

func (c Client) MakeMergeRequestUrl(user *models.User, mergeRequest *models.MergeRequest) string {
	name := c.MakeProjectName(user)
	return fmt.Sprintf("%s/%s/%s/-/merge_requests/%d", c.config.GitLab.BaseURL, c.config.GitLab.Group.Name, name, mergeRequest.IID)
}

func (c Client) MakeTaskUrl(task string) string {
	return fmt.Sprintf("%s/%s", c.config.GitLab.TaskUrlPrefix, task)
}

func (c Client) ForEachProject(callback func(project *gitlab.Project) error) error {
	options := gitlab.ListGroupProjectsOptions{}

	for {
		projects, resp, err := c.gitlab.Groups.ListGroupProjects(c.config.GitLab.Group.ID, &options)
		if err != nil {
			c.logger.Error("Failed to list projects", zap.Error(err))
			return err
		}

		errChannel := make(chan error, len(projects))

		for _, project := range projects {
			capturedProject := project
			go func() {
				errChannel <- callback(capturedProject)
			}()
		}

		for errorsLeft := len(projects); errorsLeft > 0; errorsLeft-- {
			if err = <-errChannel; err != nil {
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
