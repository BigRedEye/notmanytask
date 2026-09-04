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
		}, {
			PipelineID: 43, PipelineURL: "https://gitlab.example/pipelines/43", Task: "bench", Status: "success", Leaderboard: true,
		}},
		"CSRFToken": "csrf",
		"Pagination": adminSubmissionPagination{
			Page: 2, TotalPages: 3, Total: 125, HasPrevious: true, PreviousURL: "/admin/submissions?page=1", HasNext: true, NextURL: "/admin/submissions?page=3",
		},
		"AllURL": "/admin/submissions?kind=all", "RegularURL": "/admin/submissions?kind=regular",
		"BoardURL": "/admin/submissions?kind=leaderboard", "ResetURL": "/admin/submissions?kind=leaderboard",
	})
	if err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, expected := range []string{"table-danger", "invalid benchmark", "1.2500", "/admin/submissions/42/unban", "Page 2 of 3", "data-max-runes=\"500\""} {
		if !strings.Contains(html, expected) {
			t.Errorf("rendered admin page does not contain %q", expected)
		}
	}
}

func TestBanReasonCountsUnicodeCharacters(t *testing.T) {
	valid := []string{
		"Причина блокировки",
		strings.Repeat("я", maxBanReasonRunes),
		strings.Repeat("🙂", maxBanReasonRunes),
	}
	for _, reason := range valid {
		if !validBanReason(reason) {
			t.Errorf("valid %d-rune reason was rejected", len([]rune(reason)))
		}
	}
	invalid := []string{
		"",
		"   ",
		strings.Repeat("я", maxBanReasonRunes+1),
		strings.Repeat("🙂", maxBanReasonRunes+1),
	}
	for _, reason := range invalid {
		if validBanReason(reason) {
			t.Errorf("invalid %d-rune reason was accepted", len([]rune(reason)))
		}
	}
}

func TestAdminSubmissionsURLPreservesFiltersAndPage(t *testing.T) {
	url := adminSubmissionsURL(adminSubmissionFilters{
		Kind: "leaderboard", Group: "group with spaces", Task: "jit/fast", Login: "Алиса", State: "banned",
	}, 3)
	for _, expected := range []string{"kind=leaderboard", "group=group+with+spaces", "task=jit%2Ffast", "login=%D0%90%D0%BB%D0%B8%D1%81%D0%B0", "state=banned", "page=3"} {
		if !strings.Contains(url, expected) {
			t.Errorf("pagination URL %q does not contain %q", url, expected)
		}
	}
}
