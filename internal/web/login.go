package web

import (
	"context"
	"encoding/hex"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	perrors "github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/gitlab"
	"github.com/bigredeye/notmanytask/internal/models"
)

type loginService struct {
	webService
}

const (
	sessionKeyLogin = "login"
	sessionKeyToken = "token"
	sessionKeyOAuth = "oauthState"
)

func setupLoginService(server *server, r *gin.Engine) error {
	s := loginService{webService{server, server.config, server.logger}}

	r.GET(server.config.Endpoints.Login, s.login)
	r.GET(server.config.Endpoints.Logout, s.logout)
	r.GET(server.config.Endpoints.Signup, s.signup)
	r.POST(server.config.Endpoints.Signup, s.signupForm)
	r.GET(server.config.Endpoints.OauthCallback, s.server.validateSession, s.oauth)

	return nil
}

func (s loginService) signup(c *gin.Context) {
	c.HTML(http.StatusOK, "/signup.tmpl", gin.H{
		"CourseName": "HSE Advanced C++",
		"Config":     s.config,
	})
}

var nameRe = regexp.MustCompile("^[A-Z][a-z]+$")

func (s loginService) signupForm(c *gin.Context) {
	firstName := c.PostForm("firstname")
	lastName := c.PostForm("lastname")
	secret := c.PostForm("secret")

	log := s.log.With(
		zap.String("first_name", firstName),
		zap.String("last_name", lastName),
		zap.String("secret", secret),
	)

	log.Info("New signup request")

	if !nameRe.MatchString(firstName) {
		log.Warn("Invalid firstName from form")
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}
	if !nameRe.MatchString(lastName) {
		log.Warn("Invalid lastName from form")
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	// Find group by secret
	groupName := ""
	for _, group := range s.config.Groups {
		if secret == group.Secret {
			groupName = group.Name
		}
	}
	if groupName == "" {
		log.Warn("Unknown secret")
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}
	log = log.With(zap.String("group_name", groupName))

	user, err := s.server.db.AddUser(&models.User{
		FirstName: firstName,
		LastName:  lastName,
		GroupName: groupName,
	})
	if err != nil {
		log.Warn("Failed to add user", zap.Error(err))
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}
	if user.GitlabUser != nil {
		log.Warn("User is already registered",
			zap.Error(err),
			zap.Int("gitlab_id", user.GitlabID),
			zap.String("gitlab_login", user.GitlabLogin),
		)
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	session, err := s.server.db.CreateSession(user.ID)
	if err != nil {
		log.Error("Failed to create session", zap.Error(err))
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	storage := sessions.Default(c)
	storage.Set(sessionKeyToken, session.Token)
	err = storage.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
	}

	c.Redirect(http.StatusFound, s.config.Endpoints.Login)
}

func (s loginService) login(c *gin.Context) {
	session := sessions.Default(c)

	oauthState := uuid.New().String()
	session.Set(sessionKeyOAuth, oauthState)
	err := session.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
	}

	s.log.Info("Login", zap.String("oauth_state", oauthState))
	c.Redirect(http.StatusFound, s.server.auth.LoginURL(oauthState))
}

func (s loginService) oauth(c *gin.Context) {
	oauthState := c.Query("state")
	storage := sessions.Default(c)
	if v := storage.Get(sessionKeyOAuth); v == nil || v != oauthState {
		if v == nil {
			s.log.Info("No oauth state found")
		} else {
			s.log.Info("Mismatched oauth state", zap.String("query", oauthState), zap.String("cookie", v.(string)))
		}
		// TODO(BigRedEye): Render error to user
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		return
	}

	ctx, cancel := context.WithTimeout(c, time.Second*10)
	defer cancel()
	token, err := s.server.auth.Exchange(ctx, c.Query("code"))
	if err != nil {
		// TODO(BigRedEye): Render error to user
		s.log.Error("Failed to exchange tokens", zap.Error(err))
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Login)
		return
	}

	gitlabUser, err := gitlab.GetOAuthGitLabUser(token.AccessToken)
	if err != nil {
		s.log.Error("Failed to get gitlab user", zap.Error(err))
		// TODO(BigRedEye): Render error to user
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Login)
		return
	}
	s.log.Info("Fetched gitlab user", zap.String("gitlab_login", gitlabUser.Login), zap.Int("gitlab_id", gitlabUser.ID))

	session := c.MustGet("session").(*models.Session)
	user := c.MustGet("user").(*models.User)
	user.GitlabUser = &models.GitlabUser{
		GitlabID:    gitlabUser.ID,
		GitlabLogin: gitlabUser.Login,
	}

	err = s.server.db.SetUserGitlabAccount(session.UserID, user.GitlabUser)
	if err != nil {
		s.log.Error("Failed to set user gitlab account", zap.Error(err))
		// TODO(BigRedEye): s.RenderSignupPage("")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		return
	}

	storage.Set(sessionKeyLogin, Session{Login: user.GitlabLogin, ID: user.GitlabID})
	err = storage.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
	}

	s.server.projects.AsyncPrepareProject(user)

	c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Home)
}

func (s loginService) logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Set(sessionKeyToken, "")
	err := session.Save()
	if err != nil {
		s.log.Error("Failed to save session", zap.Error(err))
	}

	c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
}

func setupAuth(s *server, r *gin.Engine) error {
	authKey, err := hex.DecodeString(s.config.Server.Cookies.AuthenticationKey)
	if err != nil {
		return perrors.Wrap(err, "Failed to decode hex authenticationKey")
	}
	encryptKey, err := hex.DecodeString(s.config.Server.Cookies.EncryptionKey)
	if err != nil {
		return perrors.Wrap(err, "Failed to decode hex encryptionKey")
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
	storage := sessions.Default(c)
	v := storage.Get(sessionKeyToken)
	if v == nil {
		// TODO(BigRedEye): reqid
		s.logger.Info("Undefined session")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}
	token, ok := v.(string)
	if !ok {
		s.logger.Error("Failed to deserialize session token")
		storage.Set("token", "")
		storage.Save()
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}
	if token == "" {
		s.logger.Info("Empty token")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}

	user, session, err := s.db.FindUserBySession(token)
	if err != nil {
		s.logger.Warn("Failed to find session", zap.Error(err), zap.String("token", token))
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}

	c.Set("user", user)
	c.Set("session", user)

	s.logger.Info("Valid session",
		zap.Uint("session_id", session.ID),
		zap.String("gitlab_login", user.GitlabLogin),
		zap.Int("gitlab_id", user.GitlabID),
	)

	c.Next()
}
