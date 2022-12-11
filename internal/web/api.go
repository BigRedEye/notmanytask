package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/bigredeye/notmanytask/api"
	lf "github.com/bigredeye/notmanytask/internal/logfield"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type apiService struct {
	webService
}

func setupApiService(server *server, r *gin.Engine) error {
	s := apiService{webService{server, server.config, server.logger}}

	r.POST(server.config.Endpoints.Api.Report, s.report)
	r.POST(server.config.Endpoints.Api.Flag, s.createFlag)
	r.POST(server.config.Endpoints.Api.Override, s.override)
	r.POST(server.config.Endpoints.Api.ChangeGroup, s.changeGroup)
	r.GET(server.config.Endpoints.Api.Standings, s.validateToken, s.standings)
	r.GET(server.config.Endpoints.Api.ListGroupMembers, s.validateToken, s.listGroupMembers)

	return nil
}

func (s apiService) report(c *gin.Context) {
	s.log.Info("Handling grader report")
	onError := func(code int, err error) {
		s.log.Warn("Failed to process grader report", zap.Error(err))
		c.JSON(code, &api.ReportResponse{
			Status: api.Status{
				Ok:    false,
				Error: err.Error(),
			}},
		)
	}

	req := api.ReportRequest{}
	if err := c.Bind(&req); err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("failed to parse request body: %w", err))
		return
	}

	id, err := strconv.Atoi(req.PipelineID)
	if err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("failed to parse pipeline_id: %w", err))
		return
	}

	userID, err := strconv.Atoi(req.UserID)
	if err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("failed to parse user_id: %w", err))
		return
	}

	s.log.Info("Parsed report json",
		lf.Token(req.Token),
		lf.ProjectName(req.ProjectName),
		lf.GitlabID(userID),
		lf.PipelineID(id),
		zap.String("report_status", req.Status),
	)

	if !s.isTokenValid(req.Token) {
		s.log.Warn("Unknown token", lf.Token(req.Token))
		onError(http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
		return
	}

	err = s.server.pipelines.AddFresh(id, req.ProjectName)
	if err != nil {
		onError(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, &api.ReportResponse{
		Status: api.Status{
			Ok: true,
		}},
	)
}

func (s apiService) createFlag(c *gin.Context) {
	s.log.Info("Handling crasme flag request")
	onError := func(code int, err error) {
		s.log.Warn("Failed to create flag for crasme", zap.Error(err))
		c.JSON(code, &api.FlagResponse{
			Status: api.Status{
				Ok:    false,
				Error: err.Error(),
			}},
		)
	}

	req := api.FlagRequest{}
	if err := c.Bind(&req); err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("failed to parse request body: %w", err))
		return
	}

	s.log.Info("Parsed flag request json",
		lf.Token(req.Token),
		zap.String("task", req.Task),
	)

	if !s.isTokenValid(req.Token) {
		s.log.Warn("Unknown token", lf.Token(req.Token))
		onError(http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
		return
	}

	if !s.server.deadlines.AnyGroupHasTask(req.Task) {
		onError(http.StatusBadRequest, fmt.Errorf("unknown task %s", req.Task))
		return
	}

	flag, err := s.server.db.CreateFlag(req.Task)
	if err != nil {
		s.log.Error("Failed to create flag", zap.String("task", req.Task), zap.Error(err))
		onError(http.StatusInternalServerError, err)
		return
	}
	s.log.Info("Created new flag", zap.String("flag", flag.ID), zap.String("task", flag.Task))

	c.JSON(http.StatusOK, &api.FlagResponse{
		Status: api.Status{
			Ok: true,
		},
		Flag: flag.ID,
	})
}

func (s apiService) override(c *gin.Context) {
	s.log.Info("Handling score override request")
	onError := func(code int, err error) {
		s.log.Warn("Failed to override score", zap.Error(err))
		c.JSON(code, &api.OverrideResponse{
			Status: api.Status{
				Ok:    false,
				Error: err.Error(),
			}},
		)
	}

	req := api.OverrideRequest{}
	if err := c.Bind(&req); err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("failed to parse request body: %w", err))
		return
	}

	s.log.Info("Parsed override score request json",
		lf.Token(req.Token),
		zap.String("task", req.Task),
		zap.String("login", req.Login),
		zap.Int("score", req.Score),
		zap.String("status", req.Status),
	)

	if !s.isTokenValid(req.Token) {
		s.log.Warn("Unknown token", lf.Token(req.Token))
		onError(http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
		return
	}

	_, err := s.server.db.FindUserByGitlabLogin(req.Login)
	if err != nil {
		s.log.Error("Failed to get user by login", lf.GitlabLogin(req.Login))
		onError(http.StatusNotFound, fmt.Errorf("not found user"))
		return
	}

	if !s.server.deadlines.AnyGroupHasTask(req.Task) {
		onError(http.StatusBadRequest, fmt.Errorf("unknown task %s", req.Task))
		return
	}

	err = s.server.db.AddOverride(req.Login, req.Task, req.Score, req.Status)
	if err != nil {
		s.log.Error("Failed to override score", zap.String("task", req.Task), lf.GitlabLogin(req.Login), zap.Int("score", req.Score), zap.String("status", req.Status), zap.Error(err))
		onError(http.StatusInternalServerError, err)
		return
	}
	s.log.Info("Score was overriden", zap.String("task", req.Task), lf.GitlabLogin(req.Login), zap.Int("score", req.Score), zap.String("status", req.Status))

	c.JSON(http.StatusOK, &api.OverrideResponse{
		Status: api.Status{
			Ok: true,
		},
	})
}

func (s apiService) changeGroup(c *gin.Context) {
	s.log.Info("Handling change group request")
	onError := func(code int, err error) {
		s.log.Warn("Failed to change group", zap.Error(err))
		c.JSON(code, &api.ChangeGroupResponse{
			Status: api.Status{
				Ok:    false,
				Error: err.Error(),
			}},
		)
	}

	req := api.ChangeGroupRequest{}
	if err := c.Bind(&req); err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("failed to parse request body: %w", err))
		return
	}

	s.log.Info("Parsed change group request json",
		lf.Token(req.Token),
		zap.String("login", req.Login),
		zap.String("group_name", req.GroupName),
	)

	if !s.isTokenValid(req.Token) {
		s.log.Warn("Unknown token", lf.Token(req.Token))
		onError(http.StatusUnauthorized, fmt.Errorf("invalid or expired token"))
		return
	}

	user, err := s.server.db.FindUserByGitlabLogin(req.Login)
	if err != nil {
		s.log.Error("Failed to get user by login", lf.GitlabLogin(req.Login))
		onError(http.StatusNotFound, fmt.Errorf("not found user"))
		return
	}

	groupExists := false
	for _, group := range s.config.Groups {
		if group.Name == req.GroupName {
			groupExists = true
		}
	}
	if !groupExists {
		s.log.Error("Failed to find group with name", zap.String("group_name", req.GroupName))
		onError(http.StatusNotFound, fmt.Errorf("not found group"))
		return
	}
	user.GroupName = req.GroupName

	err = s.server.db.SetUserGroupName(user)
	if err != nil {
		s.log.Error("Failed to change group", lf.GitlabLogin(req.Login), zap.String("group_name", req.GroupName), zap.Error(err))
		onError(http.StatusInternalServerError, err)
		return
	}
	s.log.Info("Group was changed", lf.GitlabLogin(req.Login), zap.String("group_name", req.GroupName))

	c.JSON(http.StatusOK, &api.ChangeGroupResponse{
		Status: api.Status{
			Ok: true,
		},
	})
}

func (s apiService) userScores(c *gin.Context) {
	s.log.Info("Handling user scores request")
	onError := func(code int, err error) {
		s.log.Warn("Failed to create user scores report", zap.Error(err))
		c.JSON(code, &api.UserScoresResponse{
			Status: api.Status{
				Ok:    false,
				Error: err.Error(),
			}},
		)
	}

	req := api.UserScoresRequest{}
	if err := c.Bind(&req); err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("failed to parse request: %w", err))
		return
	}

	user, err := s.server.db.FindUserByGitlabLogin(req.Login)
	if err != nil {
		s.log.Error("Failed to get user by login", lf.GitlabLogin(req.Login))
		onError(http.StatusNotFound, fmt.Errorf("not found user"))
		return
	}

	scores, err := s.server.scorer.CalcUserScores(user)
	if err != nil {
		s.log.Error("Failed to calc scores", lf.GitlabLogin(req.Login), zap.Error(err))
		onError(http.StatusInternalServerError, fmt.Errorf("failed to calc scores"))
		return
	}

	c.JSON(http.StatusOK, &api.UserScoresResponse{
		Status: api.Status{
			Ok: true,
		},
		Scores: scores,
	})
}

func (s apiService) listGroupMembers(c *gin.Context) {
	onError := func(code int, err error) {
		s.log.Warn("Failed to list group members", zap.Error(err))
		c.JSON(code, &api.GroupMembers{
			Status: api.Status{
				Ok:    false,
				Error: err.Error(),
			}},
		)
	}

	group := c.Param("group")
	users, err := s.server.db.ListGroupUsers(group)
	if err != nil {
		s.log.Error("Failed to list group members")
		onError(http.StatusNotFound, fmt.Errorf("unknown group"))
		return
	}

	c.JSON(http.StatusOK, &api.GroupMembers{
		Status: api.Status{
			Ok: true,
		},
		Users: users,
	})
}

func (s apiService) standings(c *gin.Context) {
	s.log.Info("Handling standings request")
	onError := func(code int, err error) {
		s.log.Warn("Failed to create standings report", zap.Error(err))
		c.JSON(code, &api.StandingsResponse{
			Status: api.Status{
				Ok:    false,
				Error: err.Error(),
			}},
		)
	}

	standings, err := s.server.scorer.CalcScoreboard("hse")
	if err != nil {
		onError(http.StatusInternalServerError, fmt.Errorf("failed to list scores: %w", err))
		return
	}

	c.JSON(http.StatusOK, &api.StandingsResponse{
		Status: api.Status{
			Ok: true,
		},
		Standings: standings,
	})
}

func (s apiService) validateToken(c *gin.Context) {
	token := c.GetHeader("token")
	if !s.isTokenValid(token) {
		c.JSON(http.StatusUnauthorized, &api.Status{
			Ok:    false,
			Error: "Invalid or expired token",
		})
		c.Abort()
		return
	}
	c.Next()
}

func (s apiService) isTokenValid(token string) bool {
	for _, ttoken := range s.config.Testing.Tokens {
		if token == ttoken {
			return true
		}
	}
	return false
}
