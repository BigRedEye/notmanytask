package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTemplate creates a bare repository with one commit on main and returns
// its path, usable as a git URL.
func makeTemplate(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "template.git")
	if _, err := git("", "init", "--quiet", "--bare", "--initial-branch=main", bare); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--quiet", "--allow-empty", "-m", "init"},
		{"push", "--quiet", bare, "main"},
	} {
		if _, err := git(work, args...); err != nil {
			t.Fatal(err)
		}
	}
	return bare
}

func templateFiles(t *testing.T, bare string) []string {
	t.Helper()
	out, err := git("", "--git-dir", bare, "ls-tree", "-r", "--name-only", "main")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(out)
}

func TestPublish(t *testing.T) {
	source := makeSource(t)
	bare := makeTemplate(t)
	opts := PublishOptions{Source: source, Target: bare, Message: "Publish test"}

	result, err := Publish(PublishOptions{Source: source, Target: bare, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pushed || !strings.Contains(result.Diff, "tasks/palindrome/test.cpp") {
		t.Fatalf("dry run must show the diff and push nothing: %+v", result)
	}
	if len(templateFiles(t, bare)) != 0 {
		t.Fatal("dry run must not push")
	}

	result, err = Publish(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed {
		t.Fatalf("first publish must push: %+v", result)
	}
	files := templateFiles(t, bare)
	if len(files) != 9 || files[len(files)-1] != "tasks/palindrome/test.cpp" {
		t.Fatalf("published tree: %v", files)
	}
	if out, _ := git("", "--git-dir", bare, "log", "-1", "--format=%s", "main"); strings.TrimSpace(out) != "Publish test" {
		t.Fatalf("commit message: %q", out)
	}

	result, err = Publish(opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pushed {
		t.Fatal("nothing changed, nothing must be pushed")
	}

	// The default message names the source revision when the source is a checkout
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"add", "--all"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--quiet", "-m", "source"},
	} {
		if _, err := git(source, args...); err != nil {
			t.Fatal(err)
		}
	}
	rev, _ := git(source, "rev-parse", "--short", "HEAD")
	write(t, source, "README.md", "# course v2")
	if _, err := Publish(PublishOptions{Source: source, Target: bare}); err != nil {
		t.Fatal(err)
	}
	if out, _ := git("", "--git-dir", bare, "log", "-1", "--format=%s", "main"); !strings.HasSuffix(strings.TrimSpace(out), "from "+strings.TrimSpace(rev)) {
		t.Fatalf("default message must name the source revision: %q", out)
	}

	// Removing a task from the deadlines removes it from the template
	deadlines, _ := os.ReadFile(filepath.Join(source, "deadlines", "ami.yml"))
	write(t, source, "deadlines/ami.yml", strings.Replace(string(deadlines), "  - task: intro/aplusb\n    score: 100\n", "", 1))
	result, err = Publish(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || !strings.Contains(strings.Join(templateFiles(t, bare), "\n"), "aplusb") {
		files = templateFiles(t, bare)
		for _, f := range files {
			if strings.Contains(f, "aplusb") {
				t.Fatalf("removed task must disappear: %v", files)
			}
		}
	}
}

func TestPublishIntoEmptyTemplate(t *testing.T) {
	source := makeSource(t)
	bare := filepath.Join(t.TempDir(), "empty.git")
	if _, err := git("", "init", "--quiet", "--bare", "--initial-branch=main", bare); err != nil {
		t.Fatal(err)
	}
	result, err := Publish(PublishOptions{Source: source, Target: bare, Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || len(templateFiles(t, bare)) != 9 {
		t.Fatalf("publish into an empty template: %+v, files %v", result, templateFiles(t, bare))
	}
	// And the second publish finds the branch it just created
	if result, err = Publish(PublishOptions{Source: source, Target: bare, Branch: "main"}); err != nil || result.Pushed {
		t.Fatalf("second publish: %+v %v", result, err)
	}
}
