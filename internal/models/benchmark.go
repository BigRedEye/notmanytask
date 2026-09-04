package models

import "time"

// BenchmarkResult is a single benchmark metric reported by the grader
// together with a successful pipeline of a leaderboard task.
// Lower metric is better.
type BenchmarkResult struct {
	ID          uint   `gorm:"primaryKey"`
	GitlabLogin string `gorm:"index:idx_benchmark_login_task"`
	Task        string `gorm:"index:idx_benchmark_login_task"`
	PipelineID  int
	Metric      float64
	CreatedAt   time.Time
}
