package base

import (
	"context"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/models"
	"go.uber.org/zap"
)

type ProjectsMakerBase struct {
	Logger *zap.Logger
	Db     *database.DataBaseProxy
	Users  chan *models.User
}

type ProjectsMakerInterface interface {
	AsyncPrepareProject(user *models.User)
	Run(ctx context.Context)
	InitializeMissingProjects()
	MaybeInitializeProject(user *models.User) bool
}

func (p ProjectsMakerBase) AsyncPrepareProject(user *models.User) {
	p.Users <- user
}
