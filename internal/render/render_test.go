package render

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testManifest = `
tasks: tasks
deadlines: [deadlines/ami.yml]
export:
  include: ["cmake/**", "CMakeLists.txt", "*.md", ".gitlab-ci.yml", "deadlines/**"]
  exclude: ["**/*.secret"]
  forbid: ["Private_"]
`

const testDeadlines = `
scoring:
  policies: [{name: week, kind: linear, spec: {after: 168h, multiplier: 0}}]
  groups: [{name: default, weight: 10.0, policy: week}]
  defaultGroup: default
assignments:
- title: 01-numbers
  deadline: 10-02-2026 23:59
  tasks:
  - task: palindrome
    score: 100
  - task: intro/aplusb
    score: 100
`

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeSource(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	write(t, source, "course.yaml", testManifest)
	write(t, source, "deadlines/ami.yml", testDeadlines)
	write(t, source, "README.md", "# course")
	write(t, source, "CMakeLists.txt", "project(x)")
	write(t, source, "cmake/flags.cmake", "flags")
	write(t, source, ".gitlab-ci.yml", "grade: {}")
	write(t, source, "run_linter.sh", "not included")
	write(t, source, "private/notes.md", "staff notes")
	// Public task with private parts
	write(t, source, "tasks/palindrome/palindrome.cpp", "stub")
	write(t, source, "tasks/palindrome/README.md", "statement")
	write(t, source, "tasks/palindrome/test.cpp", "TEST_CASE(\"public\")")
	write(t, source, "tasks/palindrome/private/test.cpp", "TEST_CASE(\"Private_1\")")
	write(t, source, "tasks/palindrome/solution/palindrome.cpp", "answer")
	write(t, source, "tasks/palindrome/keys.secret", "excluded by pattern")
	// Public task with a slash in the name
	write(t, source, "tasks/intro/aplusb/aplusb.cpp", "stub")
	// Task not in the deadlines yet
	write(t, source, "tasks/future/future.cpp", "not yet")
	return source
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func TestRender(t *testing.T) {
	source := makeSource(t)
	out := t.TempDir()
	// The output looks like a checkout with stale content
	write(t, out, ".git/HEAD", "ref: refs/heads/main")
	write(t, out, "tasks/removed/removed.cpp", "stale")
	write(t, out, "README.md", "# old")
	write(t, out, "CMakeLists.txt", "project(x)")

	summary, err := Render(source, out)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		".gitlab-ci.yml",
		"CMakeLists.txt",
		"README.md",
		"cmake/flags.cmake",
		"deadlines/ami.yml",
		"tasks/intro/aplusb/aplusb.cpp",
		"tasks/palindrome/README.md",
		"tasks/palindrome/palindrome.cpp",
		"tasks/palindrome/test.cpp",
	}
	if got := listFiles(t, out); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("exported tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if _, err := os.Stat(filepath.Join(out, ".git", "HEAD")); err != nil {
		t.Fatal(".git must be left alone")
	}
	if len(summary.Tasks) != 2 || summary.Unchanged != 1 || len(summary.Updated) != 1 || len(summary.Deleted) != 1 {
		t.Fatalf("summary: %+v", summary)
	}
	if body, _ := os.ReadFile(filepath.Join(out, "README.md")); string(body) != "# course" {
		t.Fatal("changed file must be rewritten")
	}

	// A second run changes nothing
	summary, err = Render(source, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Added)+len(summary.Updated)+len(summary.Deleted) != 0 {
		t.Fatalf("second render must be a no-op: %+v", summary)
	}
}

func TestRenderRefusesLeaks(t *testing.T) {
	source := makeSource(t)
	// A private test case that slipped into a public file
	write(t, source, "tasks/palindrome/test.cpp", "TEST_CASE(\"Private_leak\")")
	out := t.TempDir()
	if _, err := Render(source, out); err == nil || !strings.Contains(err.Error(), "Private_") {
		t.Fatalf("leak must be refused, got %v", err)
	}
	if files := listFiles(t, out); len(files) != 0 {
		t.Fatalf("nothing must be written on refusal, got %v", files)
	}
}

func TestRenderMissingTask(t *testing.T) {
	source := makeSource(t)
	if err := os.RemoveAll(filepath.Join(source, "tasks", "palindrome")); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(source, t.TempDir()); err == nil || !strings.Contains(err.Error(), "palindrome") {
		t.Fatalf("a task in the deadlines without a directory must fail, got %v", err)
	}
}
