package web

import (
	"context"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	statik "github.com/rakyll/statik/fs"
	"go.uber.org/zap"

	_ "github.com/bigredeye/notmanytask/pkg/statik"
)

type server struct {
	config *Config
	logger *zap.Logger

	auth *AuthClient
}

func newServer(config *Config, logger *zap.Logger) (*server, error) {
	return &server{
		config: config,
		logger: logger,
		auth:   NewAuthClient(config),
	}, nil
}

type Session struct {
	Login string
	ID    int
}

func init() {
	gob.Register(Session{})
}

func (s *server) validateSession(c *gin.Context) {
	session := sessions.Default(c)
	v := session.Get("login")
	if v == nil {
		// TODO(BigRedEye): reqid
		s.logger.Info("Undefined session")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}
	info, ok := v.(Session)
	if !ok {
		s.logger.Error("Failed to deserialize session")
		session.Clear()
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}

	s.logger.Info("Valid session", zap.String("login", info.Login))

	c.Set("session", info)
	c.Next()
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
	authKey, err := hex.DecodeString(s.config.Server.Cookies.AuthenticationKey)
	if err != nil {
		return errors.Wrap(err, "Failed to decode hex authenticationKey")
	}
	encryptKey, err := hex.DecodeString(s.config.Server.Cookies.EncryptionKey)
	if err != nil {
		return errors.Wrap(err, "Failed to decode hex encryptionKey")
	}
	store := cookie.NewStore(authKey, encryptKey)
	store.Options(sessions.Options{
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("session", store))

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong "+fmt.Sprint(time.Now().Unix()))
	})

	r.GET(s.config.Endpoints.Signup, func(c *gin.Context) {
		c.HTML(http.StatusOK, "/signup.tmpl", gin.H{
			"CourseName": "HSE Advanced C++",
		})
	})

	r.GET(s.config.Endpoints.Login, func(c *gin.Context) {
		session := sessions.Default(c)

		oauthState := uuid.New().String()
		oauthState = "oauthSecret"
		session.Set("oauth_state", oauthState)
		session.Set("login", Session{Login: "kek123kjsdf"})
		err = session.Save()
		if err != nil {
			s.logger.Error("Failed to save session", zap.Error(err))
		}

		s.logger.Info("Login", zap.String("oauth_state", oauthState))
		c.Redirect(http.StatusTemporaryRedirect, s.auth.LoginURL(oauthState))
	})

	r.GET(s.config.Endpoints.OauthCallback, func(c *gin.Context) {
		session := sessions.Default(c)
		if v := session.Get("oauth_state"); v == nil || v != "oauthSecret" {
			// TODO(BigRedEye): Render error to user
			s.logger.Info("Invalid oauth state")
			c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
			return
		}

		ctx, cancel := context.WithTimeout(c, time.Second*5)
		defer cancel()
		token, err := s.auth.Exchange(ctx, c.Query("code"))
		if err != nil {
			// TODO(BigRedEye): Render error to user
			s.logger.Error("Failed to exchange tokens", zap.Error(err))
			c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
			return
		}

		user, err := GetGitLabUser(token.AccessToken)
		if err != nil {
			s.logger.Error("Failed to get gitlab user", zap.Error(err))
			// TODO(BigRedEye): Render error to user
			c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
			return
		}
		s.logger.Info("New user registered", zap.String("username", user.Login), zap.Int("id", user.ID))

		session.Set("login", Session{Login: user.Login, ID: user.ID})
		err = session.Save()
		if err != nil {
			s.logger.Error("Failed to save session", zap.Error(err))
		}

		// TODO: Create user repo

		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Home)
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
