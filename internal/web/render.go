package web

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/bigredeye/notmanytask/internal/scorer"
)

func (s *server) RenderSignupPage(c *gin.Context, err string) {
	c.HTML(http.StatusOK, "/signup.tmpl", gin.H{
		"CourseName":   "HSE C++ Course",
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
		"CourseName":     "HSE C++ Course",
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

func (s *server) handleChuckNorris(c *gin.Context) {
	c.Redirect(http.StatusTemporaryRedirect, "https://youtu.be/dQw4w9WgXcQ")
	return
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

type GroupLink struct {
	Link string
	Name string
}

type GroupLinks = []GroupLink

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

func (s *server) makeGroupLinks() GroupLinks {
	links := make([]GroupLink, len(s.config.Groups))

	for i, g := range s.config.Groups {
		links[i].Name = g.Name
		links[i].Link = s.makeGroupLink(g.Name)
	}

	return links
}

func (s *server) makeGroupLink(g string) string {
	return strings.Replace(s.config.Endpoints.GroupStandings, ":group", g, 1)
}

func (s *server) RenderHomePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcUserScores(user)

	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
		// FIXME(BigRedEye): Do not hardcode title
		"CourseName": "HSE C++ Course",
		"Title":      "HSE C++ Course",
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

	c.HTML(http.StatusOK, "/home.tmpl", gin.H{
		"CourseName": "HSE C++ Course",
		"Title":      "HSE C++ Course",
		"Config":     s.config,
		"Scores":     scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
	})
}

func (s *server) RedirectToStandingsPage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	c.Redirect(http.StatusMovedPermanently, s.makeGroupLink(user.GroupName))
}

func (s *server) RenderStandingsPage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcScoreboard(c.Param("group"), "")
	c.HTML(http.StatusOK, "/standings.tmpl", gin.H{
		"CourseName": "HSE C++ Course",
		"Title":      "HSE C++ Course",
		"Config":     s.config,
		"Standings":  scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
		"Groups":     s.makeGroupLinks(),
	})
}

func (s *server) RenderStandings2Page(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcScoreboard(c.Param("group"), "")
	c.HTML(http.StatusOK, "/standings2.tmpl", gin.H{
		"CourseName": "HSE C++ Course",
		"Title":      "HSE C++ Course",
		"Config":     s.config,
		"Standings":  scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
		"Groups":     s.makeGroupLinks(),
	})
}

func (s *server) RenderSubgroupStandingsPage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcScoreboard(c.Param("group"), c.Param("subgroup"))
	c.HTML(http.StatusOK, "/standings.tmpl", gin.H{
		"CourseName": "HSE C++ Course",
		"Title":      "HSE C++ Course",
		"Config":     s.config,
		"Standings":  scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
		"Groups":     s.makeGroupLinks(),
	})
}

func (s *server) RenderStandingsCheaterPage(c *gin.Context) {
	user, err := s.db.FindUserByGitlabLogin(c.Query("login"))
	scores, err := s.scorer.CalcScoreboard("students", "")
	c.HTML(http.StatusOK, "/standings.tmpl", gin.H{
		"CourseName": "HSE C++ Course",
		"Title":      "HSE C++ Course",
		"Config":     s.config,
		"Standings":  scores,
		"Error":      err,
		"Links":      s.makeLinks(user),
		"Groups":     s.makeGroupLinks(),
	})
}
