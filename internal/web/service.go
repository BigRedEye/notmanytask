package web

import (
	"github.com/bigredeye/notmanytask/internal/config"
	"go.uber.org/zap"
)

type webService struct {
	server *server
	config *config.Config
	log    *zap.Logger
}
