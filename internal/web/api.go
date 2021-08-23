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

	return nil
}

func (s apiService) report(c *gin.Context) {
	s.log.Info("Handling grader report")
	onError := func(code int, err error) {
		s.log.Warn("Failed to process grader report", zap.Error(err))
		c.JSON(code, &api.ReportResponse{
			Ok:    false,
			Error: err.Error(),
		})
	}

	req := api.ReportRequest{}
	if err := c.BindJSON(&req); err != nil {
		onError(http.StatusBadRequest, err)
		return
	}

	id, err := strconv.Atoi(req.PipelineID)
	if err != nil {
		onError(http.StatusBadRequest, err)
		return
	}

	userID, err := strconv.Atoi(req.UserID)
	if err != nil {
		onError(http.StatusBadRequest, err)
		return
	}

	s.log.Info("Parsed report json",
		lf.Token(req.Token),
		lf.ProjectName(req.ProjectName),
		lf.GitlabID(userID),
		lf.PipelineID(id),
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
	}

	err = s.server.pipelines.Fetch(id, req.ProjectName)
	if err != nil {
		onError(http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, &api.ReportResponse{
		Ok: true,
	})
}
