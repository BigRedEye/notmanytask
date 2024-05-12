package base

import (
	"context"
	"strings"
	"sync"

	"github.com/bigredeye/notmanytask/internal/database"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"go.uber.org/zap"
)

type PipelinesFetcherBase struct {
	Logger *zap.Logger
	Db     *database.DataBase

	Fresh sync.Map
}

type PipelineInfo interface{}
type Project interface{}

type PipelinesFetcherInterface interface {
	Run(ctx context.Context)
	AddFresh(id int, project string) error
	RunFresh(ctx context.Context)
	AddPipeline(projectName string, pipeline PipelineInfo) error
	Fetch(id int, project string) (PipelineInfo, error)
	FetchAllPipelines()
	FetchFreshPipelines()
	ForEachProject(callback func(project Project) error) error
}

type QualifiedPipelineID struct {
	Project string
	Id      int
}

func (p *PipelinesFetcherBase) AddFresh(id int, project string) error {
	p.Logger.Info("Added fresh pipeline", lf.ProjectName(project), lf.PipelineID(id))
	p.Fresh.Store(&QualifiedPipelineID{project, id}, true)
	return nil
}

const (
	BranchPrefix = "submits/"
)

func ParseTaskFromBranch(task string) string {
	return strings.TrimPrefix(task, BranchPrefix)
}

func MakeBranchForTask(task string) string {
	return BranchPrefix + task
}
