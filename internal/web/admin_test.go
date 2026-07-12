package web

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/config"
	"github.com/bigredeye/notmanytask/internal/deadlines"
	"github.com/bigredeye/notmanytask/internal/models"
)

func TestIsAdminUsesConfiguredGitlabLogin(t *testing.T) {
	server := &server{config: &config.Config{}}
	server.config.Server.Admins = []string{"teacher"}
	teacher := "teacher"
	student := "student"

	if !server.isAdmin(&models.User{GitlabUser: models.GitlabUser{GitlabLogin: &teacher}}) {
		t.Fatal("configured teacher was not recognized as admin")
	}
	if server.isAdmin(&models.User{GitlabUser: models.GitlabUser{GitlabLogin: &student}}) {
		t.Fatal("regular student was recognized as admin")
	}
	if server.isAdmin(nil) {
		t.Fatal("nil user was recognized as admin")
	}
}

func TestLeaderboardTemplateRendersBannedResultInRed(t *testing.T) {
	tmpl, err := buildHTMLTemplates(template.FuncMap{
		"inc":              func(i int) int { return i + 1 },
		"prettifyTaskName": filepath.Base,
		"add1":             func(f float64) float64 { return f + 1 },
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = tmpl.ExecuteTemplate(&output, "leaderboard.tmpl", map[string]interface{}{
		"Title": "Leaderboard", "CourseName": "Course", "Task": "bench", "Bonus": 1.0,
		"Deadline": deadlines.Date{Time: time.Now()},
		"Rows":     []LeaderboardRow{{Name: "Alice", Metric: 1.25, SubmittedAt: time.Now(), PipelineID: 42, Banned: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if !strings.Contains(html, `class="result-banned"`) || !strings.Contains(html, `bg-danger">Banned`) {
		t.Fatal("banned leaderboard result is not rendered with the red banned state")
	}
}

func TestAdminSubmissionsTemplateRendersBannedLeaderboardRow(t *testing.T) {
	tmpl, err := buildHTMLTemplates(template.FuncMap{
		"inc":              func(i int) int { return i + 1 },
		"prettifyTaskName": filepath.Base,
		"add1":             func(f float64) float64 { return f + 1 },
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = tmpl.ExecuteTemplate(&output, "admin_submissions.tmpl", map[string]interface{}{
		"Title":      "Admin",
		"CourseName": "Course",
		"Links":      &Links{Standings: "/standings", Admin: "/admin/submissions", Logout: "/logout"},
		"Filters":    adminSubmissionFilters{Kind: "leaderboard", State: "banned"},
		"Rows": []adminSubmissionRow{{
			PipelineID: 42, PipelineURL: "https://gitlab.example/pipelines/42", Task: "bench", Status: "success",
			Leaderboard: true, HasMetric: true, Metric: 1.25, Banned: true, BanReason: "invalid benchmark", BannedAt: time.Now(),
		}},
		"CSRFToken": "csrf",
	})
	if err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, expected := range []string{"table-danger", "invalid benchmark", "1.2500", "/admin/submissions/42/unban"} {
		if !strings.Contains(html, expected) {
			t.Errorf("rendered admin page does not contain %q", expected)
		}
	}
}

func TestAdminSubmissionFilters(t *testing.T) {
	row := adminSubmissionRow{GitlabLogin: "alice", Name: "Alice Student", Group: "hse", Task: "jit/fast", Leaderboard: true, Banned: true}
	matching := []adminSubmissionFilters{
		{Kind: "all", State: "all"},
		{Kind: "leaderboard", State: "banned", Group: "hse", Task: "jit/fast", Login: "ALI"},
		{Kind: "leaderboard", State: "banned", Login: "student"},
	}
	for _, filters := range matching {
		if !adminSubmissionMatches(row, filters) {
			t.Errorf("row unexpectedly rejected by filters: %+v", filters)
		}
	}
	notMatching := []adminSubmissionFilters{
		{Kind: "regular", State: "all"},
		{Kind: "leaderboard", State: "active"},
		{Kind: "all", State: "all", Group: "other"},
		{Kind: "all", State: "all", Login: "bob"},
	}
	for _, filters := range notMatching {
		if adminSubmissionMatches(row, filters) {
			t.Errorf("row unexpectedly accepted by filters: %+v", filters)
		}
	}
}
