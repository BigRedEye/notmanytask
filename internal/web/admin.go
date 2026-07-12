package web

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const adminCSRFSessionKey = "admin_csrf"

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
}

type adminSubmissionFilters struct {
	Kind  string
	Group string
	Task  string
	Login string
	State string
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
	}
	if filters.Kind != "regular" && filters.Kind != "leaderboard" {
		filters.Kind = "all"
	}
	if filters.State != "active" && filters.State != "banned" {
		filters.State = "all"
	}
	return filters
}

func adminSubmissionMatches(row adminSubmissionRow, filters adminSubmissionFilters) bool {
	if filters.Kind == "regular" && row.Leaderboard || filters.Kind == "leaderboard" && !row.Leaderboard {
		return false
	}
	if filters.Group != "" && row.Group != filters.Group || filters.Task != "" && row.Task != filters.Task {
		return false
	}
	if filters.State == "active" && row.Banned || filters.State == "banned" && !row.Banned {
		return false
	}
	needle := strings.ToLower(filters.Login)
	return needle == "" || strings.Contains(strings.ToLower(row.GitlabLogin), needle) || strings.Contains(strings.ToLower(row.Name), needle)
}

func (s *server) RenderAdminSubmissionsPage(c *gin.Context) {
	users, err := s.db.ListUsers()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list users")
		return
	}
	pipelines, err := s.db.ListAllPipelines()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list pipelines")
		return
	}
	benchmarks, err := s.db.ListAllBenchmarks()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list leaderboard results")
		return
	}
	bans, err := s.db.ListSubmissionBans()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list submission bans")
		return
	}
	csrfToken, err := s.adminCSRFToken(c)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to prepare form")
		return
	}

	usersByProject := make(map[string]*models.User, len(users))
	usersByID := make(map[uint]*models.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
		if project := user.GetProjectName(); project != "" {
			usersByProject[project] = user
		}
	}
	benchmarksByPipeline := make(map[int]models.BenchmarkResult, len(benchmarks))
	for _, result := range benchmarks {
		benchmarksByPipeline[result.PipelineID] = result
	}
	bansByPipeline := make(map[int]models.SubmissionBan, len(bans))
	for _, ban := range bans {
		bansByPipeline[ban.PipelineID] = ban
	}

	rows := make([]adminSubmissionRow, 0, len(pipelines))
	groupsSet := make(map[string]struct{})
	tasksSet := make(map[string]struct{})
	for _, pipeline := range pipelines {
		row := adminSubmissionRow{
			PipelineID:  pipeline.ID,
			PipelineURL: s.gitlab.MakeProjectPipelineURL(pipeline.Project, pipeline.ID),
			Project:     pipeline.Project,
			Task:        pipeline.Task,
			Status:      pipeline.Status,
			StartedAt:   pipeline.StartedAt,
		}
		if user := usersByProject[pipeline.Project]; user != nil {
			row.Name = user.FirstName + " " + user.LastName
			row.Group = user.GroupName
			if user.GitlabLogin != nil {
				row.GitlabLogin = *user.GitlabLogin
			}
			groupsSet[user.GroupName] = struct{}{}
		}
		if benchmark, ok := benchmarksByPipeline[pipeline.ID]; ok {
			row.Leaderboard = true
			row.Metric = benchmark.Metric
			row.HasMetric = true
			if row.GitlabLogin == "" {
				row.GitlabLogin = benchmark.GitlabLogin
			}
		}
		if ban, ok := bansByPipeline[pipeline.ID]; ok {
			row.Banned = true
			row.BanReason = ban.Reason
			row.BannedAt = ban.CreatedAt
			if admin := usersByID[ban.AdminUserID]; admin != nil {
				row.BannedByName = admin.FirstName + " " + admin.LastName
			}
		}
		tasksSet[pipeline.Task] = struct{}{}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].StartedAt.After(rows[j].StartedAt) })

	filters := normalizeAdminFilters(c)
	filtered := make([]adminSubmissionRow, 0, len(rows))
	for _, row := range rows {
		if adminSubmissionMatches(row, filters) {
			filtered = append(filtered, row)
		}
	}
	const maxAdminRows = 500
	truncated := len(filtered) > maxAdminRows
	if truncated {
		filtered = filtered[:maxAdminRows]
	}
	groups := sortedStringSet(groupsSet)
	tasks := sortedStringSet(tasksSet)

	c.HTML(http.StatusOK, "admin_submissions.tmpl", gin.H{
		"CourseName": s.config.Server.CourseName,
		"Title":      "Submission administration",
		"Links":      s.makeLinks(s.getUser(c)),
		"Rows":       filtered,
		"Filters":    filters,
		"Groups":     groups,
		"Tasks":      tasks,
		"CSRFToken":  csrfToken,
		"Truncated":  truncated,
	})
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
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
	if reason == "" || len(reason) > 500 {
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
	if err := s.db.UnbanSubmission(pipelineID); err != nil {
		c.String(http.StatusInternalServerError, "failed to unban submission")
		return
	}
	s.cache.Clear()
	s.logger.Info("Submission unbanned", zap.Int("pipeline_id", pipelineID), zap.Uint("admin_user_id", s.getUser(c).ID))
	c.Redirect(http.StatusSeeOther, "/admin/submissions")
}

func (row adminSubmissionRow) ModerationTitle() string {
	if !row.Banned {
		return ""
	}
	return fmt.Sprintf("Banned by %s at %s: %s", row.BannedByName, row.BannedAt.Format(time.RFC3339), row.BanReason)
}
