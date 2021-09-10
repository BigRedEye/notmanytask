package web

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/scorer"
)

func (s *server) RenderSignupPage(c *gin.Context, err string) {
	c.HTML(http.StatusOK, "/signup.tmpl", gin.H{
		"CourseName":   "HSE Advanced C++",
		"Config":       s.config,
		"ErrorMessage": err,
	})
}

func (s *server) RenderSubmitFlagPage(c *gin.Context) {
	s.RenderSubmitFlagPageDetails(c, "", "")
}

func (s *server) RenderSubmitFlagPageDetails(c *gin.Context, err string, success string) {
	user := c.MustGet("user").(*models.User)
	c.HTML(http.StatusOK, "/flag.tmpl", gin.H{
		"CourseName":     "HSE Advanced C++",
		"Config":         s.config,
		"ErrorMessage":   err,
		"SuccessMessage": success,
		"Links":          s.makeLinks(user),
	})
}

var flagRe = regexp.MustCompile(`^\{FLAG(-[a-z0-9_]+)+(-[0-9a-f]+)+\}$`)

func (s *server) handleFlagSubmit(c *gin.Context) {
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

	err := s.db.SubmitFlag(flag, *user.GitlabLogin)
	if err != nil {
		s.RenderSubmitFlagPageDetails(c, "Unknown flag", "")
		return
	}

	s.RenderSubmitFlagPageDetails(c, "", "The matrix has you...")
	return
}

func reverseScores(scores *scorer.UserScores) {
	groups := scores.Groups
	for i, j := 0, len(groups)-1; i < j; i, j = i+1, j-1 {
		groups[i], groups[j] = groups[j], groups[i]
	}
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

func (s *server) makeLinks(user *models.User) Links {
	return Links{
		Deadlines:       s.config.Endpoints.Home,
		Standings:       s.config.Endpoints.Standings,
		TasksRepository: s.config.GitLab.TaskUrlPrefix,
		Repository:      s.gitlab.MakeProjectUrl(user),
		Submits:         s.gitlab.MakeProjectSubmitsUrl(user),
		Logout:          s.config.Endpoints.Logout,
		SubmitFlag:      s.config.Endpoints.Flag,
	}
}

func (s *server) RenderHomePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcUserScores(user)
	reverseScores(scores)

	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
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
	user, err := s.db.FindUserByGitlabLogin(c.Query("login"))
	var scores *scorer.UserScores
	if err == nil {
		scores, err = s.scorer.CalcUserScores(user)
	}
	reverseScores(scores)

	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
		"CourseName": "HSE Advanced C++",
		"Title":      "HSE Advanced C++",
		"Config":     s.config,
		"Scores":     scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
	})
}

func (s *server) RenderStandingsPage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcScoreboard("hse")
	c.HTML(http.StatusOK, "/standings.tmpl", gin.H{
		"CourseName": "HSE Advanced C++",
		"Title":      "HSE Advanced C++",
		"Config":     s.config,
		"Standings":  scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
	})
}
