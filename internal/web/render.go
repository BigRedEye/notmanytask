package web

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/scorer"
	"github.com/bigredeye/notmanytask/pkg/generic"
)

func (s *server) RenderSignupPage(c *gin.Context, err string) {
	c.HTML(http.StatusOK, "signup.tmpl", gin.H{
		"CourseName":   "HSE Advanced C++",
		"Config":       s.config,
		"ErrorMessage": err,
		"Platform":     s.config.Platform.Mode,
	})
}

func (s *server) RenderTelegramLogin(c *gin.Context, err string) {
	c.HTML(http.StatusOK, "telegram.tmpl", gin.H{
		"CourseName":   "HSE Advanced C++",
		"Config":       s.config,
		"ErrorMessage": err,
	})
}

func (s *server) RenderSubmitFlagPage(c *gin.Context) {
	s.RenderSubmitFlagPageDetails(c, "", "")
}

func (s *server) RenderSubmitFlagPageDetails(c *gin.Context, err, success string) {
	user := c.MustGet("user").(*models.User)
	c.HTML(http.StatusOK, "flag.tmpl", gin.H{
		"CourseName":     "HSE Advanced C++",
		"Config":         s.config,
		"ErrorMessage":   err,
		"SuccessMessage": success,
		"Links":          s.makeLinks(user),
	})
}

var flagRe = regexp.MustCompile(`^\{FLAG(-[a-z0-9_/]+)+(-[0-9a-f]+)+\}$`)

func (s *server) handleFlagSubmitGitlab(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	if user.GitlabLogin == nil {
		s.logger.Error("User without gitlab login!", lf.UserID(user.ID))
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	flag := c.PostForm("flag")
	if !flagRe.MatchString(flag) {
		s.logger.Warn("Invalid flag", zap.String("flag", flag), lf.UserID(user.ID), lf.GitlabLogin(*user.GitlabLogin))
		s.RenderSubmitFlagPageDetails(c, "Invalid flag", "")
		return
	}

	err := s.db.SubmitFlagGitlab(flag, *user.GitlabLogin)
	if err != nil {
		s.RenderSubmitFlagPageDetails(c, "Unknown flag", "")
		return
	}

	s.RenderSubmitFlagPageDetails(c, "", "The matrix has you...")
}

func (s *server) handleFlagSubmitGitea(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	if user.GiteaLogin == nil {
		s.logger.Error("User without gitea login!", lf.UserID(user.ID))
		c.Redirect(http.StatusFound, s.config.Endpoints.Signup)
		return
	}

	flag := c.PostForm("flag")
	if !flagRe.MatchString(flag) {
		s.logger.Warn("Invalid flag", zap.String("flag", flag), lf.UserID(user.ID), lf.GiteaLogin(*user.GiteaLogin))
		s.RenderSubmitFlagPageDetails(c, "Invalid flag", "")
		return
	}

	err := s.db.SubmitFlagGitea(flag, *user.GiteaLogin)
	if err != nil {
		s.RenderSubmitFlagPageDetails(c, "Unknown flag", "")
		return
	}

	s.RenderSubmitFlagPageDetails(c, "", "The matrix has you...")
}

func (s *server) handleChuckNorris(c *gin.Context) {
	c.Redirect(http.StatusTemporaryRedirect, "https://youtu.be/dQw4w9WgXcQ")
}

func reverseScores(scores *scorer.UserScores) {
	generic.ReverseSlice(scores.Groups)
}

type Links struct {
	Deadlines       string
	Standings       string
	TasksRepository string
	Repository      string
	Submits         string
	Logout          string
	SubmitFlag      string
}

func (s *server) makeLinks(user *models.User) *Links {
	return &Links{
		Deadlines:       s.config.Endpoints.Home,
		Standings:       s.config.Endpoints.Standings,
		TasksRepository: s.config.Platform.TaskUrlPrefix,
		Repository:      s.client.MakeProjectURL(user),
		Submits:         s.client.MakeProjectSubmitsURL(user),
		Logout:          s.config.Endpoints.Logout,
		SubmitFlag:      s.config.Endpoints.Flag,
	}
}

func (s *server) RenderHomePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcUserScores(user)
	reverseScores(scores)

	c.HTML(http.StatusOK, "home.tmpl", gin.H{
		// FIXME(BigRedEye): Do not hardcode title
		"CourseName": "HSE Advanced C++",
		"Title":      "HSE Advanced C++",
		"Config":     s.config,
		"Scores":     scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
	})
}

func (s *server) RenderCheaterPage(c *gin.Context) {
	user, err := s.db.FindUserByLogin(c.Query("login"))
	var scores *scorer.UserScores
	if err == nil {
		scores, err = s.scorer.CalcUserScores(user)
	}
	reverseScores(scores)

	c.HTML(http.StatusOK, "home.tmpl", gin.H{
		"CourseName": "HSE Advanced C++",
		"Title":      "HSE Advanced C++",
		"Config":     s.config,
		"Scores":     scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
	})
}

func reverseScoreboardGroups(standings *scorer.Standings) {
	generic.ReverseSlice(standings.Deadlines.Assignments)
	for i := range standings.Users {
		reverseScores(standings.Users[i])
	}
}

func (s *server) doRenderStandingsPage(c *gin.Context, name string, filter scorer.UserFilter) {
	group := c.Query("group")
	if group == "" {
		group = "hse"
	}
	var links *Links
	if user, session, err := s.tryFindUserByToken(c); err == nil && session != nil {
		links = s.makeLinks(user)
	}

	scores, err := s.cache.Fetch(fmt.Sprintf("scores/%s/%s", group, name), time.Second*10, func() (interface{}, error) {
		scores, err := s.scorer.CalcScoreboardWithFilter(group, filter)
		reverseScoreboardGroups(scores)
		return scores, err
	})
	c.HTML(http.StatusOK, "standings.tmpl", gin.H{
		"CourseName":  "HSE Advanced C++",
		"Title":       "HSE Advanced C++",
		"Config":      s.config,
		"GroupConfig": s.config.Groups.FindGroup(group),
		"Standings":   scores.Value().(*scorer.Standings),
		"Error":       err,
		"Links":       links,
	})
}

func (s *server) RenderStandingsPage(c *gin.Context) {
	s.doRenderStandingsPage(c, "standings", nil)
}

func (s *server) RenderRetakesPage(c *gin.Context) {
	s.doRenderStandingsPage(c, "retakes", func(user *models.User) bool {
		return user.HasRetake
	})
}

func (s *server) RenderStandingsCheaterPage(c *gin.Context) {
	group := "hse"
	user, _ := s.db.FindUserByLogin(c.Query("login"))
	scores, err := s.scorer.CalcScoreboard(group)
	reverseScoreboardGroups(scores)
	c.HTML(http.StatusOK, "standings.tmpl", gin.H{
		"CourseName":  "HSE Advanced C++",
		"Title":       "HSE Advanced C++",
		"Config":      s.config,
		"GroupConfig": s.config.Groups.FindGroup(group),
		"Standings":   scores,
		"Error":       err,
		"Links":       s.makeLinks(user),
	})
}
