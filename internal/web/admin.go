package web

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bigredeye/notmanytask/internal/database"
	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	adminCSRFSessionKey = "admin_csrf"
	maxBanReasonRunes   = 500
	maxBulkPipelines    = database.DefaultAdminSubmissionsPageSize
)

type adminSubmissionRow struct {
	PipelineID   int
	PipelineURL  string
	Project      string
	Task         string
	Status       string
	StartedAt    time.Time
	Name         string
	GitlabLogin  string
	Group        string
	Leaderboard  bool
	Metric       float64
	HasMetric    bool
	Banned       bool
	BanReason    string
	BannedAt     time.Time
	BannedByName string
	History      []database.SubmissionModerationEvent
}

type adminSubmissionFilters struct {
	Kind  string
	Group string
	Task  string
	Login string
	State string
	Page  int
}

type adminSubmissionPagination struct {
	Page        int
	TotalPages  int
	Total       int64
	HasPrevious bool
	PreviousURL string
	HasNext     bool
	NextURL     string
}

func (s *server) adminCSRFToken(c *gin.Context) (string, error) {
	storage := sessions.Default(c)
	if token, ok := storage.Get(adminCSRFSessionKey).(string); ok && token != "" {
		return token, nil
	}
	token := uuid.NewString()
	storage.Set(adminCSRFSessionKey, token)
	if err := storage.Save(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *server) validateAdminCSRF(c *gin.Context) bool {
	want, _ := sessions.Default(c).Get(adminCSRFSessionKey).(string)
	got := c.PostForm("csrf_token")
	return want != "" && got != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func normalizeAdminFilters(c *gin.Context) adminSubmissionFilters {
	filters := adminSubmissionFilters{
		Kind:  c.DefaultQuery("kind", "all"),
		Group: c.Query("group"),
		Task:  c.Query("task"),
		Login: strings.TrimSpace(c.Query("login")),
		State: c.DefaultQuery("state", "all"),
		Page:  1,
	}
	if page, err := strconv.Atoi(c.Query("page")); err == nil && page > 0 {
		filters.Page = page
	}
	if filters.Kind != "regular" && filters.Kind != "leaderboard" {
		filters.Kind = "all"
	}
	if filters.State != "active" && filters.State != "banned" {
		filters.State = "all"
	}
	return filters
}

func adminSubmissionsURL(filters adminSubmissionFilters, page int) string {
	values := url.Values{}
	values.Set("kind", filters.Kind)
	if filters.Group != "" {
		values.Set("group", filters.Group)
	}
	if filters.Task != "" {
		values.Set("task", filters.Task)
	}
	if filters.Login != "" {
		values.Set("login", filters.Login)
	}
	values.Set("state", filters.State)
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	return "/admin/submissions?" + values.Encode()
}

func validBanReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	runes := utf8.RuneCountInString(reason)
	return runes >= 1 && runes <= maxBanReasonRunes
}

func parseBulkPipelineIDs(values []string) ([]int, error) {
	if len(values) == 0 || len(values) > maxBulkPipelines {
		return nil, fmt.Errorf("select 1 to %d pipelines", maxBulkPipelines)
	}
	seen := make(map[int]struct{}, len(values))
	ids := make([]int, 0, len(values))
	for _, value := range values {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			return nil, errors.New("invalid pipeline id")
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func safeAdminReturnURL(value string) string {
	if value == "/admin/submissions" || strings.HasPrefix(value, "/admin/submissions?") {
		return value
	}
	return "/admin/submissions"
}

func (s *server) RenderAdminSubmissionsPage(c *gin.Context) {
	filters := normalizeAdminFilters(c)
	page, err := s.db.ListAdminSubmissions(database.AdminSubmissionFilters{
		Kind:     filters.Kind,
		Group:    filters.Group,
		Task:     filters.Task,
		Search:   filters.Login,
		State:    filters.State,
		Page:     filters.Page,
		PageSize: database.DefaultAdminSubmissionsPageSize,
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list submissions")
		return
	}
	options, err := s.db.ListAdminSubmissionFilterOptions()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list submission filters")
		return
	}
	statistics, err := s.db.GetAdminStatistics()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load admin statistics")
		return
	}
	csrfToken, err := s.adminCSRFToken(c)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to prepare form")
		return
	}
	pipelineIDs := make([]int, 0, len(page.Items))
	for _, submission := range page.Items {
		pipelineIDs = append(pipelineIDs, submission.PipelineID)
	}
	events, err := s.db.ListSubmissionModerationEvents(pipelineIDs)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list moderation history")
		return
	}
	historyByPipeline := make(map[int][]database.SubmissionModerationEvent, len(pipelineIDs))
	for _, event := range events {
		historyByPipeline[event.PipelineID] = append(historyByPipeline[event.PipelineID], event)
	}

	rows := make([]adminSubmissionRow, 0, len(page.Items))
	for _, submission := range page.Items {
		row := adminSubmissionRow{
			PipelineID:   submission.PipelineID,
			PipelineURL:  s.gitlab.MakeProjectPipelineURL(submission.Project, submission.PipelineID),
			Project:      submission.Project,
			Task:         submission.Task,
			Status:       submission.Status,
			StartedAt:    submission.StartedAt,
			Name:         strings.TrimSpace(submission.FirstName + " " + submission.LastName),
			GitlabLogin:  submission.GitlabLogin,
			Group:        submission.GroupName,
			Leaderboard:  submission.Leaderboard,
			Banned:       submission.Banned,
			BanReason:    submission.BanReason,
			BannedByName: submission.BannedByName,
			History:      historyByPipeline[submission.PipelineID],
		}
		if submission.Metric != nil {
			row.Metric = *submission.Metric
			row.HasMetric = true
		}
		if submission.BannedAt != nil {
			row.BannedAt = *submission.BannedAt
		}
		rows = append(rows, row)
	}
	filters.Page = page.Page
	pagination := adminSubmissionPagination{
		Page:        page.Page,
		TotalPages:  page.TotalPages,
		Total:       page.Total,
		HasPrevious: page.Page > 1,
		HasNext:     page.Page < page.TotalPages,
	}
	if pagination.HasPrevious {
		pagination.PreviousURL = adminSubmissionsURL(filters, page.Page-1)
	}
	if pagination.HasNext {
		pagination.NextURL = adminSubmissionsURL(filters, page.Page+1)
	}
	allFilters := filters
	allFilters.Kind = "all"
	regularFilters := filters
	regularFilters.Kind = "regular"
	leaderboardFilters := filters
	leaderboardFilters.Kind = "leaderboard"
	resetFilters := adminSubmissionFilters{Kind: filters.Kind, State: "all", Page: 1}

	c.HTML(http.StatusOK, "admin_submissions.tmpl", gin.H{
		"CourseName": s.config.Server.CourseName,
		"Title":      "Submission administration",
		"Links":      s.makeLinks(s.getUser(c)),
		"Rows":       rows,
		"Filters":    filters,
		"Groups":     options.Groups,
		"Tasks":      options.Tasks,
		"CSRFToken":  csrfToken,
		"Pagination": pagination,
		"AllURL":     adminSubmissionsURL(allFilters, 1),
		"RegularURL": adminSubmissionsURL(regularFilters, 1),
		"BoardURL":   adminSubmissionsURL(leaderboardFilters, 1),
		"ResetURL":   adminSubmissionsURL(resetFilters, 1),
		"CurrentURL": adminSubmissionsURL(filters, page.Page),
		"Statistics": statistics,
	})
}

func (s *server) adminPipelineID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("pipeline"))
	if err != nil || id <= 0 {
		c.String(http.StatusBadRequest, "invalid pipeline id")
		return 0, false
	}
	if !s.validateAdminCSRF(c) {
		c.String(http.StatusForbidden, "invalid CSRF token")
		return 0, false
	}
	return id, true
}

func (s *server) handleAdminBanSubmission(c *gin.Context) {
	pipelineID, ok := s.adminPipelineID(c)
	if !ok {
		return
	}
	reason := strings.TrimSpace(c.PostForm("reason"))
	if !validBanReason(reason) {
		c.String(http.StatusBadRequest, "reason must contain 1 to 500 characters")
		return
	}
	if _, err := s.db.FindPipelineByID(pipelineID); err != nil {
		c.String(http.StatusNotFound, "pipeline not found")
		return
	}
	admin := s.getUser(c)
	if err := s.db.BanSubmission(pipelineID, admin.ID, reason); err != nil {
		c.String(http.StatusInternalServerError, "failed to ban submission")
		return
	}
	s.cache.Clear()
	s.logger.Info("Submission banned", zap.Int("pipeline_id", pipelineID), zap.Uint("admin_user_id", admin.ID), zap.String("reason", reason))
	c.Redirect(http.StatusSeeOther, "/admin/submissions?state=banned")
}

func (s *server) handleAdminUnbanSubmission(c *gin.Context) {
	pipelineID, ok := s.adminPipelineID(c)
	if !ok {
		return
	}
	reason := strings.TrimSpace(c.PostForm("reason"))
	if !validBanReason(reason) {
		c.String(http.StatusBadRequest, "reason must contain 1 to 500 characters")
		return
	}
	admin := s.getUser(c)
	if err := s.db.UnbanSubmission(pipelineID, admin.ID, reason); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(http.StatusNotFound, "pipeline not found")
			return
		}
		c.String(http.StatusInternalServerError, "failed to unban submission")
		return
	}
	s.cache.Clear()
	s.logger.Info("Submission unbanned", zap.Int("pipeline_id", pipelineID), zap.Uint("admin_user_id", admin.ID), zap.String("reason", reason))
	c.Redirect(http.StatusSeeOther, "/admin/submissions")
}

func (s *server) handleAdminBulkModeration(c *gin.Context) {
	if !s.validateAdminCSRF(c) {
		c.String(http.StatusForbidden, "invalid CSRF token")
		return
	}
	pipelineIDs, err := parseBulkPipelineIDs(c.PostFormArray("pipeline_id"))
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	reason := strings.TrimSpace(c.PostForm("reason"))
	if !validBanReason(reason) {
		c.String(http.StatusBadRequest, "reason must contain 1 to 500 characters")
		return
	}
	action := models.SubmissionModerationAction(c.PostForm("action"))
	if action != models.SubmissionModerationActionBan && action != models.SubmissionModerationActionUnban {
		c.String(http.StatusBadRequest, "invalid moderation action")
		return
	}
	admin := s.getUser(c)
	changed, err := s.db.ModerateSubmissions(pipelineIDs, admin.ID, action, reason)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(http.StatusNotFound, "pipeline not found")
			return
		}
		c.String(http.StatusInternalServerError, "failed to moderate submissions")
		return
	}
	s.cache.Clear()
	s.logger.Info("Bulk submission moderation applied", zap.String("action", string(action)), zap.Ints("pipeline_ids", pipelineIDs), zap.Int("changed", changed), zap.Uint("admin_user_id", admin.ID), zap.String("reason", reason))
	c.Redirect(http.StatusSeeOther, safeAdminReturnURL(c.PostForm("return_to")))
}

func (row adminSubmissionRow) ModerationTitle() string {
	if !row.Banned {
		return ""
	}
	return fmt.Sprintf("Banned by %s at %s: %s", row.BannedByName, row.BannedAt.Format(time.RFC3339), row.BanReason)
}
