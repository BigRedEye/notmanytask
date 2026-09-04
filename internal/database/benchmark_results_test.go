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

func TestBenchmarkResultMigrationAndUpsertPostgres(t *testing.T) {
	dsn := os.Getenv("NMT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set NMT_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	schema := "nmt_benchmark_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err := tx.AutoMigrate(&models.BenchmarkResult{}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	legacy := []models.BenchmarkResult{
		{GitlabLogin: "alice", Task: "bench", PipelineID: 501, Metric: 2.0, CreatedAt: now.Add(-time.Minute)},
		{GitlabLogin: "alice", Task: "bench", PipelineID: 501, Metric: 1.5, CreatedAt: now},
	}
	if err := tx.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateBenchmarkResults(tx); err != nil {
		t.Fatal(err)
	}
	if err := migrateBenchmarkResults(tx); err != nil {
		t.Fatal(err)
	}

	db := &DataBase{DB: tx}
	assertBenchmark(t, tx, 501, "alice", "bench", 1.5, 1)
	if err := db.AddBenchmarkResult(&models.BenchmarkResult{GitlabLogin: "alice", Task: "bench", PipelineID: 501, Metric: 1.25, CreatedAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	assertBenchmark(t, tx, 501, "alice", "bench", 1.25, 1)

	if err := db.AddBenchmarkResult(&models.BenchmarkResult{GitlabLogin: "bob", Task: "bench-v2", PipelineID: 501, Metric: 0.75, CreatedAt: now.Add(2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	assertBenchmark(t, tx, 501, "bob", "bench-v2", 0.75, 1)
	if !tx.Migrator().HasIndex(&models.BenchmarkResult{}, "idx_benchmark_results_pipeline_id_unique") {
		t.Fatal("unique pipeline index was not created")
	}
}

func assertBenchmark(t *testing.T, db *gorm.DB, pipelineID int, login, task string, metric float64, wantCount int64) {
	t.Helper()
	var count int64
	if err := db.Model(&models.BenchmarkResult{}).Where("pipeline_id = ?", pipelineID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != wantCount {
		t.Fatalf("pipeline %d has %d benchmark rows, want %d", pipelineID, count, wantCount)
	}
	var result models.BenchmarkResult
	if err := db.First(&result, "pipeline_id = ?", pipelineID).Error; err != nil {
		t.Fatal(err)
	}
	if result.GitlabLogin != login || result.Task != task || result.Metric != metric {
		t.Fatalf("unexpected benchmark result: %+v", result)
	}
}
