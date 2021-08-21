package web

import (
	"context"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/gitlab"
	"github.com/bigredeye/notmanytask/internal/models"
)

type loginService struct {
	webService
}

func setupLoginService(server *server, r *gin.Engine) error {
	s := loginService{webService{server, server.config, server.logger}}

	r.GET(server.config.Endpoints.Signup, s.signup)
	r.POST(server.config.Endpoints.Signup, s.signupForm)
	r.GET(server.config.Endpoints.Login, s.login)
	r.GET(server.config.Endpoints.OauthCallback, s.oauth)
	r.GET(server.config.Endpoints.Logout, s.logout)

	return nil
}

func (s loginService) signup(c *gin.Context) {
	c.HTML(http.StatusOK, "/signup.tmpl", gin.H{
		"CourseName": "HSE Advanced C++",
	})
}

func (s loginService) signupForm(c *gin.Context) {
	firstName := c.PostForm("firstname")
	lastName := c.PostForm("lastname")
	groupName := c.PostForm("secret")

	// TODO(BigRedEye): Validate form

	session := sessions.Default(c)
	session.Set("register", RegisterInfo{
		FirstName: firstName,
		LastName:  lastName,
		GroupName: groupName,
	})
	err := session.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
	}

	c.Redirect(http.StatusFound, s.config.Endpoints.Login)
}

func (s loginService) login(c *gin.Context) {
	session := sessions.Default(c)

	oauthState := uuid.New().String()
	session.Set("oauth_state", oauthState)
	err := session.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
	}

	s.log.Info("Login", zap.String("oauth_state", oauthState))
	c.Redirect(http.StatusTemporaryRedirect, s.server.auth.LoginURL(oauthState))
}

func (s loginService) oauth(c *gin.Context) {
	oauthState := c.Query("state")
	session := sessions.Default(c)
	if v := session.Get("oauth_state"); v == nil || v != oauthState {
		if v == nil {
			s.log.Info("No oauth state found")
		} else {
			s.log.Info("Mismatched oauth state", zap.String("query", oauthState), zap.String("cookie", v.(string)))
		}
		// TODO(BigRedEye): Render error to user
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		return
	}

	// TODO(BigRedEye): Move to separate function
	v := session.Get("register")
	if v == nil {
		s.log.Info("No register info found")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}
	info, ok := v.(RegisterInfo)
	if !ok {
		s.log.Error("Failed to load register info")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}

	ctx, cancel := context.WithTimeout(c, time.Second*5)
	defer cancel()
	token, err := s.server.auth.Exchange(ctx, c.Query("code"))
	if err != nil {
		// TODO(BigRedEye): Render error to user
		s.log.Error("Failed to exchange tokens", zap.Error(err))
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		return
	}

	user, err := gitlab.GetOAuthGitLabUser(token.AccessToken)
	if err != nil {
		s.log.Error("Failed to get gitlab user", zap.Error(err))
		// TODO(BigRedEye): Render error to user
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		return
	}
	s.log.Info("Fetched gitlab user", zap.String("username", user.Login), zap.Int("id", user.ID))

	session.Set("login", Session{Login: user.Login, ID: user.ID})
	err = session.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
	}

	err = s.server.db.AddUser(&models.User{
		ID:        user.ID,
		Login:     user.Login,
		FirstName: info.FirstName,
		LastName:  info.LastName,
		GroupName: info.GroupName,
	})
	if err != nil {
		s.log.Error("Failed to add user", zap.Error(err))
		// TODO(BigRedEye): s.RenderSignupPage("")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
	}

	// TODO: Create user repo

	c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Home)
}

func (s loginService) logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Set("login", Session{})
	err := session.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
	}

	c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
}

func setupAuth(s *server, r *gin.Engine) error {
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
	return nil
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
	if info.Login == "" {
		s.logger.Info("Empty session")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}

	s.logger.Info("Valid session", zap.String("login", info.Login), zap.Int("id", info.ID))

	c.Set("session", info)
	c.Next()
}
