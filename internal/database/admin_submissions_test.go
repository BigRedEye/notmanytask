package database

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNormalizeAdminPagination(t *testing.T) {
	tests := []struct {
		page, size     int
		wantPage, want int
	}{
		{0, 0, 1, DefaultAdminSubmissionsPageSize},
		{-5, 10, 1, 10},
		{3, MaxAdminSubmissionsPageSize + 1, 3, MaxAdminSubmissionsPageSize},
	}
	for _, test := range tests {
		page, size := normalizeAdminPagination(test.page, test.size)
		if page != test.wantPage || size != test.want {
			t.Errorf("normalizeAdminPagination(%d, %d) = (%d, %d), want (%d, %d)", test.page, test.size, page, size, test.wantPage, test.want)
		}
	}
}

func TestListAdminSubmissionsPostgres(t *testing.T) {
	dsn := os.Getenv("NMT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set NMT_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	schema := "nmt_admin_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := root.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer root.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")

	tx := root.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := tx.Exec("SET LOCAL search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.AutoMigrate(&models.User{}, &models.Pipeline{}, &models.BenchmarkResult{}, &models.SubmissionBan{}); err != nil {
		t.Fatal(err)
	}
	db := &DataBase{DB: tx}

	aliceLogin, bobLogin, teacherLogin := "alice", "bob", "teacher"
	aliceRepo, bobRepo, teacherRepo := "https://gitlab.example/course/alice-project", "https://gitlab.example/course/bob-project", "https://gitlab.example/course/teacher-project"
	users := []*models.User{
		{GitlabUser: models.GitlabUser{GitlabLogin: &aliceLogin, Repository: &aliceRepo}, FirstName: "Алиса", LastName: "Студент", GroupName: "hse"},
		{GitlabUser: models.GitlabUser{GitlabLogin: &bobLogin, Repository: &bobRepo}, FirstName: "Боб", LastName: "Студент", GroupName: "hse"},
		{GitlabUser: models.GitlabUser{GitlabLogin: &teacherLogin, Repository: &teacherRepo}, FirstName: "Тест", LastName: "Учитель", GroupName: "staff"},
	}
	for _, user := range users {
		if err := tx.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if users[0].ProjectName != "alice-project" || users[1].ProjectName != "bob-project" {
		t.Fatalf("project names were not initialized: %q, %q", users[0].ProjectName, users[1].ProjectName)
	}
	// The indexed project identity, rather than the display/clone URL format,
	// must drive pipeline ownership joins.
	if err := tx.Model(users[0]).UpdateColumn("repository", "ssh://git@gitlab.example/course/url-no-longer-matches").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Model(users[1]).UpdateColumn("project_name", "").Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillUserProjectNames(tx); err != nil {
		t.Fatal(err)
	}
	var backfilled models.User
	if err := tx.First(&backfilled, users[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if backfilled.ProjectName != "bob-project" {
		t.Fatalf("backfilled project name = %q, want bob-project", backfilled.ProjectName)
	}
	if !tx.Migrator().HasIndex(&models.User{}, "ProjectName") {
		t.Fatal("users.project_name index was not created")
	}
	newTeacherRepo := "https://gitlab.example/course/teacher-renamed"
	users[2].Repository = &newTeacherRepo
	if err := db.SetUserRepository(users[2]); err != nil {
		t.Fatal(err)
	}
	if users[2].ProjectName != "teacher-renamed" {
		t.Fatalf("SetUserRepository project name = %q, want teacher-renamed", users[2].ProjectName)
	}
	now := time.Now().UTC().Truncate(time.Second)
	pipelines := []models.Pipeline{
		{ID: 1, Project: "alice-project", Task: "bench", Status: models.PipelineStatusSuccess, StartedAt: now.Add(-time.Minute)},
		{ID: 2, Project: "alice-project", Task: "regular", Status: models.PipelineStatusSuccess, StartedAt: now.Add(-2 * time.Minute)},
		{ID: 3, Project: "bob-project", Task: "bench", Status: models.PipelineStatusSuccess, StartedAt: now.Add(-3 * time.Minute)},
		{ID: 4, Project: "bob-project", Task: "regular", Status: models.PipelineStatusFailed, StartedAt: now.Add(-4 * time.Minute)},
		{ID: 5, Project: "alice-project", Task: "regular", Status: models.PipelineStatusFailed, StartedAt: now.Add(-5 * time.Minute)},
	}
	if err := tx.Create(&pipelines).Error; err != nil {
		t.Fatal(err)
	}
	benchmarks := []models.BenchmarkResult{
		{GitlabLogin: aliceLogin, Task: "bench", PipelineID: 1, Metric: 1.25, CreatedAt: now},
		// A retry for the same pipeline verifies that the lateral join still returns one row.
		{GitlabLogin: aliceLogin, Task: "bench", PipelineID: 1, Metric: 1.20, CreatedAt: now.Add(time.Second)},
		{GitlabLogin: bobLogin, Task: "bench", PipelineID: 3, Metric: 2.5, CreatedAt: now},
	}
	if err := tx.Create(&benchmarks).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateBenchmarkResults(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Create(&models.SubmissionBan{PipelineID: 1, AdminUserID: users[2].ID, Reason: "invalid environment", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}

	t.Run("database filters", func(t *testing.T) {
		page, err := db.ListAdminSubmissions(AdminSubmissionFilters{
			Kind: "leaderboard", Group: "hse", Task: "bench", Search: "АЛИС", State: "banned", Page: 1, PageSize: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].PipelineID != 1 {
			t.Fatalf("unexpected filtered page: %+v", page)
		}
		item := page.Items[0]
		if item.Metric == nil || *item.Metric != 1.20 || !item.Leaderboard || !item.Banned || item.BannedByName != "Тест Учитель" {
			t.Fatalf("joined submission data is incomplete: %+v", item)
		}

		literalWildcard, err := db.ListAdminSubmissions(AdminSubmissionFilters{Search: "%", PageSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if literalWildcard.Total != 0 {
			t.Fatalf("LIKE wildcard was not escaped: got %d matches", literalWildcard.Total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		first, err := db.ListAdminSubmissions(AdminSubmissionFilters{Page: 1, PageSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		if first.Total != 5 || first.TotalPages != 3 || len(first.Items) != 2 || first.Items[0].PipelineID != 1 || first.Items[1].PipelineID != 2 {
			t.Fatalf("unexpected first page: %+v", first)
		}
		last, err := db.ListAdminSubmissions(AdminSubmissionFilters{Page: 99, PageSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		if last.Page != 3 || len(last.Items) != 1 || last.Items[0].PipelineID != 5 {
			t.Fatalf("unexpected clamped last page: %+v", last)
		}
	})

	t.Run("filter options", func(t *testing.T) {
		options, err := db.ListAdminSubmissionFilterOptions()
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(options.Groups) != "[hse staff]" || fmt.Sprint(options.Tasks) != "[bench regular]" {
			t.Fatalf("unexpected filter options: %+v", options)
		}
	})
}
