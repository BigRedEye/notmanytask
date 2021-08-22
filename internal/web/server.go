package web

import (
	"encoding/gob"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	statik "github.com/rakyll/statik/fs"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/gitlab"
	_ "github.com/bigredeye/notmanytask/pkg/statik"
)

type server struct {
	config *config.Config
	logger *zap.Logger

	auth      *AuthClient
	db        *database.DataBase
	deadlines *deadlines.Fetcher
	projects  *gitlab.ProjectsMaker
}

func newServer(config *config.Config, logger *zap.Logger, db *database.DataBase, deadlines *deadlines.Fetcher, projects *gitlab.ProjectsMaker) (*server, error) {
	return &server{
		config:    config,
		logger:    logger,
		auth:      NewAuthClient(config),
		db:        db,
		deadlines: deadlines,
		projects:  projects,
	}, nil
}

type Session struct {
	Login string
	ID    int
}

type RegisterInfo struct {
	FirstName string
	LastName  string
	GroupName string
}

func init() {
	gob.Register(Session{})
	gob.Register(RegisterInfo{})
}

func buildHTMLTemplates(hfs http.FileSystem, funcMap template.FuncMap) (*template.Template, error) {
	tmpl := template.New("").Funcs(funcMap)
	err := statik.Walk(hfs, "/", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		fmt.Printf("Path %s\n", path)

		if !info.IsDir() {
			bytes, err := statik.ReadFile(hfs, path)
			if err != nil {
				return err
			}

			template.Must(tmpl.New(path).Parse(string(bytes)))
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to collect html templates")
	}

	return tmpl, nil
}

func (s *server) run() error {
	statikFS, err := statik.New()
	if err != nil {
		return errors.Wrap(err, "Failed to open statik fs")
	}
	tmpl, err := buildHTMLTemplates(statikFS, make(template.FuncMap))
	if err != nil {
		return errors.Wrap(err, "Failed to build html templates")
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(ginzap.Ginzap(s.logger, time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(s.logger, true))

	r.SetHTMLTemplate(tmpl)

	// TODO(BigRedEye): Move cookies to the separate file
	setupAuth(s, r)
	setupLoginService(s, r)

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong "+fmt.Sprint(time.Now().Unix()))
	})

	r.GET(s.config.Endpoints.Home, s.validateSession, func(c *gin.Context) {
		c.HTML(http.StatusOK, "/home.tmpl", gin.H{
			"CourseName": "HSE Advanced C++",
		})
	})

	r.StaticFS("/static", statikFS)

	s.logger.Info("Starting server", zap.String("bind_address", s.config.Server.ListenAddress))
	return r.Run(s.config.Server.ListenAddress)
}
