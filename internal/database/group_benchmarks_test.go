package database

import (
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

// Only results of successful pipelines of the reported task count, and a
// banned pipeline drops out of the leaderboard.
func TestListGroupBenchmarksPostgres(t *testing.T) {
	dsn := os.Getenv("NMT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set NMT_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	schema := "nmt_leaderboard_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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

	login, repo := "alice", "https://gitlab.example/course/alice"
	if err := tx.Create(&models.User{GitlabUser: models.GitlabUser{GitlabLogin: &login, Repository: &repo}, FirstName: "Alice", LastName: "A", GroupName: "hse"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	pipelines := []models.Pipeline{
		{ID: 1, Project: "alice", Task: "bench", Status: models.PipelineStatusSuccess, StartedAt: now.Add(-3 * time.Hour)}, // banned below
		{ID: 2, Project: "alice", Task: "bench", Status: models.PipelineStatusSuccess, StartedAt: now.Add(-2 * time.Hour)},
		{ID: 3, Project: "alice", Task: "bench", Status: models.PipelineStatusFailed, StartedAt: now.Add(-time.Hour)},
		{ID: 4, Project: "alice", Task: "plain", Status: models.PipelineStatusSuccess, StartedAt: now},
	}
	if err := tx.Create(&pipelines).Error; err != nil {
		t.Fatal(err)
	}
	for id, metric := range map[int]float64{1: 0.5, 2: 1.5, 3: 0.1, 4: 0.2} {
		if err := db.AddBenchmarkResult(&models.BenchmarkResult{GitlabLogin: login, Task: "bench", PipelineID: id, Metric: metric}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.BanSubmission(1, "cheating", ""); err != nil {
		t.Fatal(err)
	}

	results, err := db.ListGroupBenchmarks("hse")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].PipelineID != 2 || results[0].Metric != 1.5 || results[0].FirstName != "Alice" {
		t.Fatalf("unexpected group benchmarks: %+v", results)
	}
	if other, err := db.ListGroupBenchmarks("mipt"); err != nil || len(other) != 0 {
		t.Fatalf("another group sees the results: %+v, %v", other, err)
	}
}
