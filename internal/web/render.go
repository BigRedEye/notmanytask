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
		"CourseName":   s.config.Server.CourseName,
		"Config":       s.config,
		"ErrorMessage": err,
	})
}

func (s *server) RenderTelegramLogin(c *gin.Context, err string) {
	c.HTML(http.StatusOK, "telegram.tmpl", gin.H{
		"CourseName":   s.config.Server.CourseName,
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
		"CourseName":     s.config.Server.CourseName,
		"Config":         s.config,
		"ErrorMessage":   err,
		"SuccessMessage": success,
		"Links":          s.makeLinks(user),
	})
}

var flagRe = regexp.MustCompile(`^\{FLAG(-[a-z0-9_/]+)+(-[0-9a-f]+)+\}$`)

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
	links := &Links{
		Deadlines:       s.config.Endpoints.Home,
		Standings:       s.config.Endpoints.Standings,
		TasksRepository: s.config.GitLab.TaskUrlPrefix,
		Logout:          s.config.Endpoints.Logout,
		SubmitFlag:      s.config.Endpoints.Flag,
	}
	// No repository yet (still being created): no links into nowhere
	if user != nil && user.Repository != nil {
		links.Repository = s.gitlab.MakeProjectURL(user)
		links.Submits = s.gitlab.MakeProjectSubmitsURL(user)
	}
	return links
}

func (s *server) RenderHomePage(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	scores, err := s.scorer.CalcUserScores(user)
	reverseScores(scores)

	c.HTML(http.StatusOK, "home.tmpl", gin.H{
		// FIXME(BigRedEye): Do not hardcode title
		"CourseName": s.config.Server.CourseName,
		"Title":      s.config.Server.CourseName,
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

	c.HTML(http.StatusOK, "home.tmpl", gin.H{
		"CourseName": s.config.Server.CourseName,
		"Title":      s.config.Server.CourseName,
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

// GroupLink is one entry of the group switcher above the standings table.
type GroupLink struct {
	Name   string
	Link   string
	Active bool
}

// standingsLink builds the pretty standings path: /standings/<group> or
// /standings/<group>/<subgroup>.
func (s *server) standingsLink(group, subgroup string) string {
	link := s.config.Endpoints.Standings + "/" + group
	if subgroup != "" {
		link += "/" + subgroup
	}
	return link
}

func (s *server) makeGroupLinks(active string) []GroupLink {
	links := make([]GroupLink, 0, len(s.config.Groups))
	for _, group := range s.config.Groups {
		links = append(links, GroupLink{Name: group.Name, Link: s.standingsLink(group.Name, ""), Active: group.Name == active})
	}
	return links
}

func (s *server) doRenderStandingsPage(c *gin.Context, name string, filter scorer.UserFilter) {
	// Pretty paths /standings/<group>[/<subgroup>] first, ?group=&subgroup= for old links
	group := c.Param("group")
	if group == "" {
		group = c.Query("group")
	}
	if group == "" {
		defaul := s.config.Groups.FindDefaultGroup()
		if defaul != nil {
			group = defaul.Name
		}
	}
	groupConfig := s.config.Groups.FindGroup(group)

	// Optional subgroup filter; unknown subgroups are ignored
	subgroup := c.Param("subgroup")
	if subgroup == "" {
		subgroup = c.Query("subgroup")
	}
	if groupConfig == nil || groupConfig.FindSubgroup(subgroup) == nil {
		subgroup = ""
	}
	if subgroup != "" {
		userFilter := filter
		filter = func(user *models.User) bool {
			return user.SubgroupName == subgroup && (userFilter == nil || userFilter(user))
		}
	}

	var links *Links
	if user, session, err := s.tryFindUserByToken(c); err == nil && session != nil {
		links = s.makeLinks(user)
	}

	scores, err := s.cache.Fetch(fmt.Sprintf("scores/%s/%s/%s", group, subgroup, name), time.Second*10, func() (interface{}, error) {
		scores, err := s.scorer.CalcScoreboardWithFilter(group, filter)
		reverseScoreboardGroups(scores)
		return scores, err
	})
	c.HTML(http.StatusOK, "standings.tmpl", gin.H{
		"CourseName":  s.config.Server.CourseName,
		"Title":       s.config.Server.CourseName,
		"Config":      s.config,
		"GroupConfig": groupConfig,
		"Group":       group,
		"Subgroup":    subgroup,
		"Groups":      s.makeGroupLinks(group),
		"GroupLink":   s.standingsLink(group, ""),
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
	user, _ := s.db.FindUserByGitlabLogin(c.Query("login"))
	scores, err := s.scorer.CalcScoreboard(group)
	reverseScoreboardGroups(scores)
	c.HTML(http.StatusOK, "standings.tmpl", gin.H{
		"CourseName":  s.config.Server.CourseName,
		"Title":       s.config.Server.CourseName,
		"Config":      s.config,
		"GroupConfig": s.config.Groups.FindGroup(group),
		"Standings":   scores,
		"Error":       err,
		"Links":       s.makeLinks(user),
	})
}
