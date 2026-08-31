//go:build integration

package database

import (
	"context"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/models"
)

var (
	testDatabaseName = regexp.MustCompile(`^nmt_test_[0-9a-f]{32}$`)
	testDatabaseUser = regexp.MustCompile(`^nmt_user_[0-9a-f]{32}$`)
)

func guardedTestURL(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.Fragment != "" || u.RawPath != "" {
		t.Fatal("TEST_DATABASE_URL rejected")
	}
	password, hasPassword := u.User.Password()
	host, portText, err := net.SplitHostPort(u.Host)
	port, portErr := strconv.Atoi(portText)
	query, queryErr := url.ParseQuery(u.RawQuery)
	if !hasPassword || password == "" || host != "127.0.0.1" || err != nil || portErr != nil || port < 1 || port > 65535 ||
		!testDatabaseUser.MatchString(u.User.Username()) || len(u.Path) < 2 || !testDatabaseName.MatchString(u.Path[1:]) ||
		queryErr != nil || len(query) != 1 || len(query["sslmode"]) != 1 || query.Get("sslmode") != "disable" {
		t.Fatal("TEST_DATABASE_URL rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config, err := pgx.ParseConfig(raw)
	if err != nil {
		t.Fatal("database guard configuration failed")
	}
	config.ConnectTimeout = 5 * time.Second
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal("database guard connection failed")
	}
	defer conn.Close(context.Background())
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal("database guard transaction failed")
	}
	defer tx.Rollback(context.Background())
	var database, user, marker string
	err = tx.QueryRow(ctx, `SELECT current_database(), current_user, COALESCE(shobj_description(oid, 'pg_database'), '') FROM pg_database WHERE datname=current_database()`).Scan(&database, &user, &marker)
	if err != nil || database != u.Path[1:] || user != u.User.Username() || marker != "notmanytask-disposable-test:v1" {
		t.Fatal("database identity guard failed")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("database guard commit failed")
	}
	return raw
}

func TestOpenDataBase(t *testing.T) {
	raw := guardedTestURL(t)
	store, err := OpenDataBase(zap.NewNop(), raw)
	if err != nil {
		t.Fatal("OpenDataBase failed")
	}
	sqlDB, err := store.DB.DB()
	if err != nil {
		t.Fatal("SQL database handle failed")
	}
	defer sqlDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal("database ping failed")
	}
	for _, model := range []interface{}{&models.User{}, &models.Pipeline{}, &models.Session{}, &models.Flag{}, &models.OverriddenScore{}, &models.MergeRequest{}} {
		if !store.Migrator().HasTable(model) {
			t.Fatal("AutoMigrate table missing")
		}
	}

	createdAt := time.Unix(100, 0).UTC()
	mergeRequest := &models.MergeRequest{
		ID:                    1,
		IID:                   2,
		Project:               "project",
		Task:                  "task",
		State:                 models.MergeRequestStateOpened,
		StartedAt:             createdAt,
		LastPipelineStatus:    models.PipelineStatusPending,
		LastPipelineCreatedAt: createdAt,
	}
	if err := store.UpsertMergeRequest(mergeRequest); err != nil {
		t.Fatal("merge request insert failed")
	}
	mergeRequest.State = models.MergeRequestStateMerged
	mergeRequest.UserNotesCount = 1
	mergeRequest.LastPipelineStatus = models.PipelineStatusSuccess
	if err := store.UpsertMergeRequest(mergeRequest); err != nil {
		t.Fatal("merge request update failed")
	}

	projectRows, err := store.ListProjectMergeRequests("project")
	if err != nil || len(projectRows) != 1 {
		t.Fatal("project merge request query failed")
	}
	got := projectRows[0]
	if got.State != models.MergeRequestStateMerged || got.UserNotesCount != 1 ||
		got.LastPipelineStatus != models.PipelineStatusSuccess {
		t.Fatal("merge request update was not persisted")
	}
	allRows, err := store.ListAllMergeRequests()
	if err != nil || len(allRows) != 1 {
		t.Fatal("all merge request query failed")
	}
}
