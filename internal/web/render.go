package web

import (
	"net/http"

	"github.com/gin-gonic/gin"

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

func reverseScores(scores *scorer.UserScores) {
	groups := scores.Groups
	for i, j := 0, len(groups)-1; i < j; i, j = i+1, j-1 {
		groups[i], groups[j] = groups[j], groups[i]
	}
}

type Links struct {
	TasksRepository string
	Repository      string
	Submits         string
	Logout          string
}

func (s *server) RenderHomePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcScores(user)
	reverseScores(scores)

	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
		// FIXME(BigRedEye): Do not hardcode title
		"CourseName": "HSE Advanced C++",
		"Title":      "HSE Advanced C++",
		"Config":     s.config,
		"Scores":     scores,
		"Error":      err,
		"Links": Links{
			TasksRepository: s.config.GitLab.TaskUrlPrefix,
			Repository:      s.gitlab.MakeProjectUrl(user),
			Submits:         s.gitlab.MakeProjectSubmitsUrl(user),
			Logout:          s.config.Endpoints.Logout,
		},
	})
}

func (s *server) RenderCheaterPage(c *gin.Context) {
	user, err := s.db.FindUserByGitlabLogin(c.Query("login"))
	var scores *scorer.UserScores
	if err == nil {
		scores, err = s.scorer.CalcScores(user)
	}
	reverseScores(scores)

	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
		"CourseName": "HSE Advanced C++",
		"Config":     s.config,
		"Scores":     scores,
		"Error":      err,
	})
}
