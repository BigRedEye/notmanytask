package database

import (
	"os"
	"strings"
	"testing"

	"github.com/bigredeye/notmanytask/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBenchmarkResultUpsertPostgres(t *testing.T) {
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

	// A retry of the grader report replaces the metric of the same pipeline.
	db := &DataBase{DB: tx}
	for _, metric := range []float64{2.0, 1.5} {
		if err := db.AddBenchmarkResult(&models.BenchmarkResult{GitlabLogin: "alice", Task: "bench", PipelineID: 501, Metric: metric}); err != nil {
			t.Fatal(err)
		}
	}
	var results []models.BenchmarkResult
	if err := tx.Find(&results, "pipeline_id = ?", 501).Error; err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Metric != 1.5 {
		t.Fatalf("unexpected benchmark results: %+v", results)
	}
}
