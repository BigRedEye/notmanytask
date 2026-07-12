package database

import (
	"errors"
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

func TestSubmissionModerationPostgres(t *testing.T) {
	dsn := os.Getenv("NMT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set NMT_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	schema := "nmt_moderation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err := tx.AutoMigrate(&models.User{}, &models.Pipeline{}, &models.SubmissionBan{}, &models.SubmissionModerationEvent{}); err != nil {
		t.Fatal(err)
	}
	db := &DataBase{DB: tx}

	adminLogin := "teacher"
	admin := &models.User{GitlabUser: models.GitlabUser{GitlabLogin: &adminLogin}, FirstName: "Test", LastName: "Teacher"}
	if err := tx.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	pipelines := []models.Pipeline{
		{ID: 101, Project: "alice-project", Task: "bench", Status: models.PipelineStatusSuccess, StartedAt: time.Now()},
		{ID: 102, Project: "bob-project", Task: "bench", Status: models.PipelineStatusSuccess, StartedAt: time.Now()},
		{ID: 103, Project: "carol-project", Task: "bench", Status: models.PipelineStatusSuccess, StartedAt: time.Now()},
	}
	if err := tx.Create(&pipelines).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.BanSubmission(101, admin.ID, "invalid environment"); err != nil {
		t.Fatal(err)
	}
	if err := db.BanSubmission(101, admin.ID, "duplicate request"); err != nil {
		t.Fatal(err)
	}
	assertModerationCounts(t, tx, 101, 1, 1)

	if err := db.UnbanSubmission(101, admin.ID, "environment verified"); err != nil {
		t.Fatal(err)
	}
	if err := db.UnbanSubmission(101, admin.ID, "duplicate request"); err != nil {
		t.Fatal(err)
	}
	assertModerationCounts(t, tx, 101, 0, 2)

	if err := db.BanSubmission(101, admin.ID, "new violation"); err != nil {
		t.Fatal(err)
	}
	assertModerationCounts(t, tx, 101, 1, 3)

	events, err := db.ListSubmissionModerationEvents([]int{101})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Action != models.SubmissionModerationActionBan || events[0].Reason != "new violation" || events[0].PreviousBanned || !events[0].CurrentBanned {
		t.Fatalf("unexpected latest event: %+v", events[0])
	}
	if events[1].Action != models.SubmissionModerationActionUnban || events[1].Reason != "environment verified" || !events[1].PreviousBanned || events[1].CurrentBanned {
		t.Fatalf("unexpected unban event: %+v", events[1])
	}
	if events[2].Action != models.SubmissionModerationActionBan || events[2].Reason != "invalid environment" || events[2].PreviousBanned || !events[2].CurrentBanned {
		t.Fatalf("unexpected initial event: %+v", events[2])
	}
	if events[0].AdminName != "Test Teacher" {
		t.Fatalf("admin name = %q, want %q", events[0].AdminName, "Test Teacher")
	}

	legacyCreatedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	if err := tx.Create(&models.SubmissionBan{PipelineID: 102, AdminUserID: admin.ID, Reason: "legacy ban", CreatedAt: legacyCreatedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillSubmissionModerationEvents(tx); err != nil {
		t.Fatal(err)
	}
	if err := backfillSubmissionModerationEvents(tx); err != nil {
		t.Fatal(err)
	}
	assertModerationCounts(t, tx, 102, 1, 1)
	legacyEvents, err := db.ListSubmissionModerationEvents([]int{102})
	if err != nil {
		t.Fatal(err)
	}
	if len(legacyEvents) != 1 || legacyEvents[0].Reason != "legacy ban" || legacyEvents[0].CreatedAt.UTC() != legacyCreatedAt {
		t.Fatalf("unexpected backfilled event: %+v", legacyEvents)
	}

	changed, err := db.ModerateSubmissions([]int{103, 101, 103}, admin.ID, models.SubmissionModerationActionBan, "bulk violation")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("bulk ban changed %d submissions, want 1", changed)
	}
	assertModerationCounts(t, tx, 101, 1, 3)
	assertModerationCounts(t, tx, 103, 1, 1)

	changed, err = db.ModerateSubmissions([]int{103, 101, 102}, admin.ID, models.SubmissionModerationActionUnban, "bulk review complete")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 3 {
		t.Fatalf("bulk unban changed %d submissions, want 3", changed)
	}
	assertModerationCounts(t, tx, 101, 0, 4)
	assertModerationCounts(t, tx, 102, 0, 2)
	assertModerationCounts(t, tx, 103, 0, 2)

	if _, err := db.ModerateSubmissions([]int{101, 999999}, admin.ID, models.SubmissionModerationActionBan, "must roll back"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing pipeline error = %v, want record not found", err)
	}
	assertModerationCounts(t, tx, 101, 0, 4)
}

func assertModerationCounts(t *testing.T, db *gorm.DB, pipelineID, wantBans, wantEvents int64) {
	t.Helper()
	var bans, events int64
	if err := db.Model(&models.SubmissionBan{}).Where("pipeline_id = ?", pipelineID).Count(&bans).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.SubmissionModerationEvent{}).Where("pipeline_id = ?", pipelineID).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if bans != wantBans || events != wantEvents {
		t.Fatalf("pipeline %d has %d bans/%d events, want %d/%d", pipelineID, bans, events, wantBans, wantEvents)
	}
}
