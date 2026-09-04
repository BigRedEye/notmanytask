// Package render builds the public tree of a course from its private source:
// the tasks listed in the deadlines files, without private directories, plus
// the files the manifest includes. The output directory is made to match
// exactly (stale files are deleted), so it can be a checkout of the public
// template; committing and pushing is left to git.
package render

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/bigredeye/notmanytask/internal/deadlines"
)

const ManifestName = "course.yaml"

// Manifest is course.yaml at the root of the private course repository.
type Manifest struct {
	// Tasks is the directory with one subdirectory per task
	Tasks string `yaml:"tasks"`
	// Deadlines files define which tasks are public: a task is exported iff
	// it is listed in at least one of them
	Deadlines       []string `yaml:"deadlines"`
	DeadlinesFormat string   `yaml:"deadlinesFormat"`
	Export          struct {
		// Include: files outside the tasks that are exported (globs)
		Include []string `yaml:"include"`
		// Exclude: never exported, checked after everything else (globs).
		// `private` and `solution` directories are excluded always.
		Exclude []string `yaml:"exclude"`
		// Forbid: substrings that must not appear in any exported file,
		// e.g. the prefix of private test cases
		Forbid []string `yaml:"forbid"`
	} `yaml:"export"`
}

// privateDirs are never exported, whatever the manifest says.
var privateDirs = map[string]bool{"private": true, "solution": true}

type Summary struct {
	Tasks     []string
	Added     []string
	Updated   []string
	Deleted   []string
	Unchanged int
}

func (s Summary) String() string {
	return fmt.Sprintf("%d tasks, %d added, %d updated, %d deleted, %d unchanged",
		len(s.Tasks), len(s.Added), len(s.Updated), len(s.Deleted), s.Unchanged)
}

func LoadManifest(source string) (*Manifest, error) {
	body, err := os.ReadFile(filepath.Join(source, ManifestName))
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to read %s", ManifestName)
	}
	manifest := &Manifest{}
	if err := yaml.UnmarshalStrict(body, manifest); err != nil {
		return nil, errors.Wrapf(err, "Failed to parse %s", ManifestName)
	}
	if manifest.Tasks == "" {
		manifest.Tasks = "tasks"
	}
	if manifest.DeadlinesFormat == "" {
		manifest.DeadlinesFormat = "v2"
	}
	if len(manifest.Deadlines) == 0 {
		return nil, errors.Errorf("%s: deadlines must list at least one file", ManifestName)
	}
	return manifest, nil
}

// publicTasks collects the tasks listed in the deadlines files.
func publicTasks(source string, manifest *Manifest) ([]string, error) {
	set := make(map[string]bool)
	for _, file := range manifest.Deadlines {
		body, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(file)))
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to read deadlines %s", file)
		}
		parsed, err := deadlines.Parse(body, manifest.DeadlinesFormat)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to parse deadlines %s", file)
		}
		for _, group := range parsed.Assignments {
			for _, task := range group.Tasks {
				set[task.Task] = true
			}
		}
	}
	tasks := make([]string, 0, len(set))
	for task := range set {
		tasks = append(tasks, task)
	}
	sort.Strings(tasks)
	return tasks, nil
}

type rules struct {
	tasksDir string
	tasks    []string
	include  []glob
	exclude  []glob
}

// exported decides whether a slash-separated path relative to the source
// root goes to the public tree.
func (r rules) exported(rel string) bool {
	for _, segment := range strings.Split(rel, "/") {
		if privateDirs[segment] {
			return false
		}
	}
	if anyMatch(r.exclude, rel) {
		return false
	}
	for _, task := range r.tasks {
		if strings.HasPrefix(rel, path.Join(r.tasksDir, task)+"/") {
			return true
		}
	}
	return anyMatch(r.include, rel)
}

// Plan lists the files to export, relative slash-separated paths in order.
func Plan(source string, manifest *Manifest) ([]string, []string, error) {
	tasks, err := publicTasks(source, manifest)
	if err != nil {
		return nil, nil, err
	}
	for _, task := range tasks {
		if _, err := os.Stat(filepath.Join(source, filepath.FromSlash(manifest.Tasks), filepath.FromSlash(task))); err != nil {
			return nil, nil, errors.Errorf("task %s is in the deadlines but not in %s/", task, manifest.Tasks)
		}
	}
	include, err := compileGlobs(manifest.Export.Include)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Bad export.include pattern")
	}
	exclude, err := compileGlobs(manifest.Export.Exclude)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Bad export.exclude pattern")
	}
	r := rules{tasksDir: strings.Trim(manifest.Tasks, "/"), tasks: tasks, include: include, exclude: exclude}

	files := []string{}
	err = filepath.WalkDir(source, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == ManifestName || !d.Type().IsRegular() {
			return nil
		}
		if r.exported(rel) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to walk the source")
	}
	sort.Strings(files)
	return tasks, files, nil
}

// checkLeaks refuses files that contain a forbidden substring.
func checkLeaks(source string, files []string, forbid []string) error {
	if len(forbid) == 0 {
		return nil
	}
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		for _, needle := range forbid {
			if bytes.Contains(body, []byte(needle)) {
				return errors.Errorf("%s contains forbidden %q", rel, needle)
			}
		}
	}
	return nil
}

// Render makes out contain exactly the public tree of source. A .git
// directory in out is left alone.
func Render(source, out string) (*Summary, error) {
	manifest, err := LoadManifest(source)
	if err != nil {
		return nil, err
	}
	tasks, files, err := Plan(source, manifest)
	if err != nil {
		return nil, err
	}
	if err := checkLeaks(source, files, manifest.Export.Forbid); err != nil {
		return nil, errors.Wrap(err, "Refusing to export")
	}

	summary := &Summary{Tasks: tasks}
	wanted := make(map[string]bool, len(files))
	for _, rel := range files {
		wanted[rel] = true
		src := filepath.Join(source, filepath.FromSlash(rel))
		dst := filepath.Join(out, filepath.FromSlash(rel))
		body, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		existing, readErr := os.ReadFile(dst)
		switch {
		case readErr == nil && bytes.Equal(existing, body):
			summary.Unchanged++
			continue
		case readErr == nil:
			summary.Updated = append(summary.Updated, rel)
		default:
			summary.Added = append(summary.Added, rel)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, body, info.Mode().Perm()); err != nil {
			return nil, err
		}
	}

	// Delete what is not wanted any more, then empty directories
	var stale []string
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(out, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == ".git" && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !wanted[rel] {
			stale = append(stale, rel)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to walk the output")
	}
	for _, rel := range stale {
		if err := os.Remove(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			return nil, err
		}
		summary.Deleted = append(summary.Deleted, rel)
	}
	if err := removeEmptyDirs(out); err != nil {
		return nil, err
	}
	return summary, nil
}

func removeEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" && p != root {
				return filepath.SkipDir
			}
			if p != root {
				dirs = append(dirs, p)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Deepest first
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(dir); err != nil {
				return err
			}
		}
	}
	return nil
}
