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
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
)

type loginService struct {
	webService
}

const (
	sessionKeyToken = "token"
	sessionKeyOAuth = "oauthState"
)

func setupLoginService(server *server, r *gin.Engine) error {
	s := loginService{webService{server, server.config, server.logger}}

	r.GET(server.config.Endpoints.Login, s.login)
	r.GET(server.config.Endpoints.Logout, s.logout)
	r.GET(server.config.Endpoints.Signup, s.signup)
	r.POST(server.config.Endpoints.Signup, s.signupForm)
	r.GET(server.config.Endpoints.OauthCallback, s.oauth)

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
	if user.GitlabID != nil || user.GitlabLogin != nil {
		log.Warn("User is already registered",
			zap.Error(err),
			zap.Intp("gitlab_id", user.GitlabID),
			zap.Stringp("gitlab_login", user.GitlabLogin),
		)
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	if err = s.fillSessionForUser(c, user); err != nil {
		log.Error("Failed to create session", zap.Error(err))
		// renderer.RenderError();
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
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
	// Compare oauth state in query and cookie
	oauthState := c.Query("state")
	storage := sessions.Default(c)
	if v := storage.Get(sessionKeyOAuth); v == nil || v != oauthState {
		if v == nil {
			s.log.Info("No oauth state found")
		} else {
			s.log.Info("Mismatched oauth state", zap.String("query", oauthState), zap.String("cookie", v.(string)))
		}
		// TODO(BigRedEye): Render error to user
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	// Get users and session from database
	user, _, err := s.server.tryFindUserByToken(c)
	if err != nil {
		s.log.Error("Failed to find user session", zap.Error(err))
		// TODO(BigRedEye): Render error to user
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	// Resolve gitlab user
	ctx, cancel := context.WithTimeout(c, time.Second*10)
	defer cancel()
	token, err := s.server.auth.Exchange(ctx, c.Query("code"))
	if err != nil {
		// TODO(BigRedEye): Render error to user
		s.log.Error("Failed to exchange tokens", zap.Error(err))
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}
	gitlabUser, err := gitlab.GetOAuthGitLabUser(token.AccessToken)
	if err != nil {
		s.log.Error("Failed to get gitlab user", zap.Error(err))
		// TODO(BigRedEye): Render error to user
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}
	s.log.Info("Fetched gitlab user", zap.String("gitlab_login", gitlabUser.Login), zap.Int("gitlab_id", gitlabUser.ID))

	// user == nil iff the token was not provided
	// This may happen after /logout and /login
	if user == nil {
		user, err = s.server.db.FindUserByGitlabID(gitlabUser.ID)
		if err != nil {
			s.log.Error("Unknown user", zap.Error(err), zap.Int("gitlab_id", gitlabUser.ID))
			// TODO(BigRedEye): Render error to user
			c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
			return
		}
	}

	if user.GitlabLogin != nil && user.GitlabID != nil {
		if err = s.fillSessionForUser(c, user); err != nil {
			s.log.Error("Failed to create session", zap.Error(err), zap.Int("gitlab_id", gitlabUser.ID))
			// renderer.RenderError();
			c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
			return
		}
		s.log.Info("Filled session for existing user", lf.UserID(user.ID), lf.GitlabLogin(gitlabUser.Login), lf.GitlabID(gitlabUser.ID))
		c.Redirect(http.StatusFound, s.config.Endpoints.Home)
		return
	}

	user.GitlabUser = models.GitlabUser{
		GitlabID:    &gitlabUser.ID,
		GitlabLogin: &gitlabUser.Login,
	}

	err = s.server.db.SetUserGitlabAccount(user.ID, &user.GitlabUser)
	if err != nil {
		s.log.Error("Failed to set user gitlab account", zap.Error(err))
		// TODO(BigRedEye): s.RenderSignupPage("")
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		return
	}

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

func tryGetToken(c *gin.Context) *string {
	storage := sessions.Default(c)
	v := storage.Get(sessionKeyToken)
	if v == nil {
		return nil
	}
	res, _ := v.(string)
	if len(res) == 0 {
		return nil
	}
	return &res
}

func (s *server) tryFindUserByToken(c *gin.Context) (*models.User, *models.Session, error) {
	token := tryGetToken(c)
	if token == nil {
		s.logger.Info("No token found")
		return nil, nil, nil
	}

	user, session, err := s.db.FindUserBySession(*token)
	if err != nil {
		s.logger.Warn("Failed to find session", zap.Error(err), zap.Stringp("token", token))
		return nil, nil, err
	}

	return user, session, nil
}

func (s *server) validateSession(c *gin.Context) {
	user, session, err := s.tryFindUserByToken(c)
	if err != nil || session == nil {
		s.logger.Warn("Failed to find user session", zap.Error(err))
		c.Redirect(http.StatusTemporaryRedirect, s.config.Endpoints.Signup)
		c.Abort()
		return
	}

	c.Set("user", user)
	c.Set("session", session)

	s.logger.Info("Valid session",
		zap.Uint("session_id", session.ID),
		zap.Uint("user_id", user.ID),
		zap.Stringp("gitlab_login", user.GitlabLogin),
		zap.Intp("gitlab_id", user.GitlabID),
	)

	if user.GitlabID == nil || user.GitlabLogin == nil {
		s.logger.Warn("Found user without gitlab account, redirecting to /login",
			zap.String("token", session.Token),
			zap.Uint("user_id", user.ID),
		)
		c.Redirect(http.StatusFound, s.config.Endpoints.Login)
		c.Abort()
		return
	}

	c.Next()
}

func (s loginService) fillSessionForUser(c *gin.Context, user *models.User) error {
	session, err := s.server.db.CreateSession(user.ID)
	if err != nil {
		return err
	}

	storage := sessions.Default(c)
	storage.Set(sessionKeyToken, session.Token)
	return storage.Save()
}

func (s *server) getUser(c *gin.Context) *models.User {
	return c.MustGet("user").(*models.User)
}

func (s *server) getSession(c *gin.Context) *models.Session {
	return c.MustGet("session").(*models.Session)
}
