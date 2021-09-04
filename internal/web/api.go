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
		onError(http.StatusBadRequest, fmt.Errorf("Failed to parse request body: %w", err))
		return
	}

	id, err := strconv.Atoi(req.PipelineID)
	if err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("Failed to parse pipeline_id: %w", err))
		return
	}

	userID, err := strconv.Atoi(req.UserID)
	if err != nil {
		onError(http.StatusBadRequest, fmt.Errorf("Failed to parse user_id: %w", err))
		return
	}

	s.log.Info("Parsed report json",
		lf.Token(req.Token),
		lf.ProjectName(req.ProjectName),
		lf.GitlabID(userID),
		lf.PipelineID(id),
		zap.String("report_status", req.Status),
	)

	// Check token
	found := false
	for _, token := range s.config.Testing.Tokens {
		if token == req.Token {
			found = true
			break
		}
	}
	if !found {
		s.log.Warn("Unknown token", lf.Token(req.Token))
		onError(http.StatusUnauthorized, fmt.Errorf("Invalid or expired token"))
		return
	}

	err = s.server.pipelines.Fetch(id, req.ProjectName)
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
		onError(http.StatusBadRequest, fmt.Errorf("Failed to parse request body: %w", err))
		return
	}

	s.log.Info("Parsed flag request json",
		lf.Token(req.Token),
		zap.String("task", req.Task),
	)

	// Check token
	found := false
	for _, token := range s.config.Testing.Tokens {
		if token == req.Token {
			found = true
			break
		}
	}
	if !found {
		s.log.Warn("Unknown token", lf.Token(req.Token))
		onError(http.StatusUnauthorized, fmt.Errorf("Invalid or expired token"))
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
		onError(http.StatusBadRequest, fmt.Errorf("Failed to parse request: %w", err))
		return
	}

	user, err := s.server.db.FindUserByGitlabLogin(req.Login)
	if err != nil {
		s.log.Error("Failed to get user by login", lf.GitlabLogin(req.Login))
		onError(http.StatusNotFound, fmt.Errorf("Not found user"))
		return
	}

	scores, err := s.server.scorer.CalcUserScores(user)
	if err != nil {
		s.log.Error("Failed to calc scores", lf.GitlabLogin(req.Login), zap.Error(err))
		onError(http.StatusInternalServerError, fmt.Errorf("Failed to calc scores"))
		return
	}

	c.JSON(http.StatusOK, &api.UserScoresResponse{
		Status: api.Status{
			Ok: true,
		},
		Scores: scores,
	})
}
