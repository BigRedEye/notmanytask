package interfaces

import "github.com/bigredeye/notmanytask/internal/models"

type GitHostingService interface {
	ProjectNameFactory
	InitializeRepo(user *models.User) error
	MakeRepoCIRunsURL(user *models.User) string
	MakeFullRepoName(repoName string) string
	GetCIRun(repoName string, pipelineID int) (*models.Pipeline, error)
	ListRepoCIRuns(repo *models.Repo) ([]*models.Pipeline, error)
	ListAllRepos() ([]*models.Repo, error)
}

type ProjectNameFactory interface {
	MakeRepoURL(user *models.User) string
	MakeRepoName(user *models.User) string
	MakeCIRunURL(user *models.User, pipeline *models.Pipeline) string
	MakeBranchURL(user *models.User, pipeline *models.Pipeline) string
	MakeTaskURL(task string) string
}
