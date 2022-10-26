package web

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/karlseguin/ccache/v2"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/gitlab"
	"github.com/bigredeye/notmanytask/internal/scorer"
	"github.com/bigredeye/notmanytask/web"
)

type server struct {
	config *config.Config
	logger *zap.Logger

	auth      *AuthClient
	db        *database.DataBase
	deadlines *deadlines.Fetcher
	projects  *gitlab.ProjectsMaker
	pipelines *gitlab.PipelinesFetcher
	scorer    *scorer.Scorer
	gitlab    *gitlab.Client

	cache *ccache.Cache
}

func newServer(
	config *config.Config,
	logger *zap.Logger,
	db *database.DataBase,
	deadlines *deadlines.Fetcher,
	projects *gitlab.ProjectsMaker,
	pipelines *gitlab.PipelinesFetcher,
	scorer *scorer.Scorer,
	gitlab *gitlab.Client,
) (*server, error) {
	return &server{
		config:    config,
		logger:    logger,
		auth:      NewAuthClient(config),
		db:        db,
		deadlines: deadlines,
		projects:  projects,
		pipelines: pipelines,
		scorer:    scorer,
		gitlab:    gitlab,
		cache:     ccache.New(ccache.Configure()),
	}, nil
}

func buildHTMLTemplates(funcMap template.FuncMap) (*template.Template, error) {
	tmpl := template.New("").Funcs(funcMap)
	return tmpl.ParseFS(web.StaticTemplates, "*.tmpl")
}

func (s *server) run() error {
	funcs := template.FuncMap{
		"inc": func(i int) int {
			return i + 1
		},
		"prettifyTaskName": filepath.Base,
	}
	tmpl, err := buildHTMLTemplates(funcs)
	if err != nil {
		return errors.Wrap(err, "Failed to build html templates")
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(ginzap.Ginzap(s.logger, time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(s.logger, true))

	s.logger.Info("Defined templates are: ", zap.String("tmpl", tmpl.DefinedTemplates()))
	r.SetHTMLTemplate(tmpl)

	// TODO(BigRedEye): Move cookies to the separate file
	err = setupAuth(s, r)
	if err != nil {
		return errors.Wrap(err, "Failed to setup auth")
	}
	err = setupLoginService(s, r)
	if err != nil {
		return errors.Wrap(err, "Failed to setup login service")
	}
	err = setupApiService(s, r)
	if err != nil {
		return errors.Wrap(err, "Failed to setup api service")
	}

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong "+fmt.Sprint(time.Now().Unix()))
	})

	r.GET(s.config.Endpoints.Home, s.validateSession(true), s.RenderHomePage)
	r.GET(s.config.Endpoints.Flag, s.validateSession(true), s.RenderSubmitFlagPage)
	r.GET(s.config.Endpoints.Retakes, s.validateSession(true), s.RenderRetakesPage)
	r.POST(s.config.Endpoints.Flag, s.validateSession(true), s.handleFlagSubmit)
	r.GET(s.config.Endpoints.Standings /* no need to validate session */, s.RenderStandingsPage)
	r.GET("/private/solutions/:group/:task", s.handleChuckNorris)

	r.StaticFS("/static", http.FS(web.StaticContent))

	s.logger.Info("Starting server", zap.String("bind_address", s.config.Server.ListenAddress))
	return r.Run(s.config.Server.ListenAddress)
}
