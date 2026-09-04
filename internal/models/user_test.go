package models

import "testing"

func TestUserGetProjectNamePrefersStoredIdentity(t *testing.T) {
	repository := "https://gitlab.example/course/url-project/"
	user := &User{GitlabUser: GitlabUser{Repository: &repository, ProjectName: "stored-project"}}
	if got := user.GetProjectName(); got != "stored-project" {
		t.Fatalf("GetProjectName() = %q, want stored-project", got)
	}

	user.ProjectName = ""
	if got := user.GetProjectName(); got != "url-project" {
		t.Fatalf("legacy GetProjectName() = %q, want url-project", got)
	}
	if got := ProjectNameFromRepository("  "); got != "" {
		t.Fatalf("empty repository produced project %q", got)
	}
}
