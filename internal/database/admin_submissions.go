package database

import (
	"strings"
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
	"gorm.io/gorm"
)

const (
	DefaultAdminSubmissionsPageSize = 50
	MaxAdminSubmissionsPageSize     = 100
)

type AdminSubmissionFilters struct {
	Kind     string
	Group    string
	Task     string
	Search   string
	State    string
	Page     int
	PageSize int
}

type AdminSubmission struct {
	PipelineID   int
	Project      string
	Task         string
	Status       string
	StartedAt    time.Time
	FirstName    string
	LastName     string
	GitlabLogin  string
	GroupName    string
	Leaderboard  bool
	Metric       *float64
	Banned       bool
	BanReason    string
	BannedAt     *time.Time
	BannedByName string
}

type AdminSubmissionsPage struct {
	Items      []AdminSubmission
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
}

type AdminSubmissionFilterOptions struct {
	Groups []string
	Tasks  []string
}

func normalizeAdminPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultAdminSubmissionsPageSize
	}
	if pageSize > MaxAdminSubmissionsPageSize {
		pageSize = MaxAdminSubmissionsPageSize
	}
	return page, pageSize
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func (db *DataBase) adminSubmissionsQuery(filters AdminSubmissionFilters) *gorm.DB {
	query := db.Table("pipelines AS p").
		Joins(`LEFT JOIN users AS u
               ON u.deleted_at IS NULL
              AND u.project_name = p.project`).
		Joins(`LEFT JOIN LATERAL (
                 SELECT b.pipeline_id, b.gitlab_login, b.metric
                   FROM benchmark_results AS b
                  WHERE b.pipeline_id = p.id
                  ORDER BY b.created_at DESC, b.id DESC
                  LIMIT 1
               ) AS br ON TRUE`).
		Joins("LEFT JOIN submission_bans AS sb ON sb.pipeline_id = p.id").
		Joins("LEFT JOIN users AS au ON au.id = sb.admin_user_id AND au.deleted_at IS NULL")

	switch filters.Kind {
	case "regular":
		query = query.Where("br.pipeline_id IS NULL")
	case "leaderboard":
		query = query.Where("br.pipeline_id IS NOT NULL")
	}
	if filters.Group != "" {
		query = query.Where("u.group_name = ?", filters.Group)
	}
	if filters.Task != "" {
		query = query.Where("p.task = ?", filters.Task)
	}
	switch filters.State {
	case "active":
		query = query.Where("sb.pipeline_id IS NULL")
	case "banned":
		query = query.Where("sb.pipeline_id IS NOT NULL")
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		pattern := "%" + escapeLike(strings.ToLower(search)) + "%"
		query = query.Where(`(
            LOWER(COALESCE(u.gitlab_login, br.gitlab_login, '')) LIKE ? ESCAPE '\'
            OR LOWER(CONCAT_WS(' ', u.first_name, u.last_name)) LIKE ? ESCAPE '\'
        )`, pattern, pattern)
	}
	return query
}

func (db *DataBase) ListAdminSubmissions(filters AdminSubmissionFilters) (*AdminSubmissionsPage, error) {
	page, pageSize := normalizeAdminPagination(filters.Page, filters.PageSize)

	var total int64
	if err := db.adminSubmissionsQuery(filters).Count(&total).Error; err != nil {
		return nil, err
	}
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	items := make([]AdminSubmission, 0, pageSize)
	err := db.adminSubmissionsQuery(filters).
		Select(`
            p.id AS pipeline_id,
            p.project,
            p.task,
            p.status,
            p.started_at,
            COALESCE(u.first_name, '') AS first_name,
            COALESCE(u.last_name, '') AS last_name,
            COALESCE(u.gitlab_login, br.gitlab_login, '') AS gitlab_login,
            COALESCE(u.group_name, '') AS group_name,
            (br.pipeline_id IS NOT NULL) AS leaderboard,
            br.metric,
            (sb.pipeline_id IS NOT NULL) AS banned,
            COALESCE(sb.reason, '') AS ban_reason,
            sb.created_at AS banned_at,
            CONCAT_WS(' ', au.first_name, au.last_name) AS banned_by_name
        `).
		Order("p.started_at DESC, p.id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Scan(&items).Error
	if err != nil {
		return nil, err
	}

	return &AdminSubmissionsPage{
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (db *DataBase) ListAdminSubmissionFilterOptions() (*AdminSubmissionFilterOptions, error) {
	groups := make([]string, 0)
	if err := db.Model(&models.User{}).
		Where("repository IS NOT NULL AND group_name <> ''").
		Distinct().
		Order("group_name").
		Pluck("group_name", &groups).Error; err != nil {
		return nil, err
	}
	tasks := make([]string, 0)
	if err := db.Model(&models.Pipeline{}).
		Where("task <> ''").
		Distinct().
		Order("task").
		Pluck("task", &tasks).Error; err != nil {
		return nil, err
	}
	return &AdminSubmissionFilterOptions{Groups: groups, Tasks: tasks}, nil
}
