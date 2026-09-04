package render

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// PublishOptions drive Publish: the private source, the git URL of the
// public template and the branch students fork from.
type PublishOptions struct {
	Source  string
	Target  string
	Branch  string
	Message string
	// DryRun renders and shows the diff without committing or pushing
	DryRun bool
	// Author of the publish commit
	AuthorName  string
	AuthorEmail string
}

// PublishResult reports what happened; Diff is `git diff --cached` of the
// would-be commit.
type PublishResult struct {
	Summary *Summary
	Diff    string
	Pushed  bool
}

// Publish clones the target, renders the source into the clone and pushes
// the result as one commit. Nothing is pushed when the public tree did not
// change. Authentication is git's own business (ssh key, token in the URL).
func Publish(opts PublishOptions) (*PublishResult, error) {
	if opts.Branch == "" {
		opts.Branch = "main"
	}
	if opts.Message == "" {
		opts.Message = "Publish " + time.Now().Format("2006-01-02 15:04")
		// Point back at the source revision when the source is a checkout
		if rev, err := git(opts.Source, "rev-parse", "--short", "HEAD"); err == nil {
			opts.Message += " from " + strings.TrimSpace(rev)
		}
	}
	if opts.AuthorName == "" {
		opts.AuthorName = "notmanytask"
	}
	if opts.AuthorEmail == "" {
		opts.AuthorEmail = "notmanytask@localhost"
	}

	clone, err := os.MkdirTemp("", "nmt-publish-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(clone)

	if err := checkout(clone, opts.Target, opts.Branch); err != nil {
		return nil, err
	}

	summary, err := Render(opts.Source, clone)
	if err != nil {
		return nil, err
	}
	if _, err := git(clone, "add", "--all"); err != nil {
		return nil, err
	}
	diff, err := git(clone, "diff", "--cached", "--stat")
	if err != nil {
		return nil, err
	}
	result := &PublishResult{Summary: summary, Diff: diff}
	if strings.TrimSpace(diff) == "" || opts.DryRun {
		return result, nil
	}

	if _, err := git(clone, "-c", "user.name="+opts.AuthorName, "-c", "user.email="+opts.AuthorEmail,
		"commit", "--quiet", "--message", opts.Message); err != nil {
		return nil, errors.Wrap(err, "Failed to commit")
	}
	// Never force: student forks update from this branch
	if _, err := git(clone, "push", "--quiet", "origin", "HEAD:"+opts.Branch); err != nil {
		return nil, errors.Wrap(err, "Failed to push")
	}
	result.Pushed = true
	return result, nil
}

// checkout fetches the branch of the target into dir, or starts the branch
// from scratch when the target has no such branch yet (a fresh, empty
// template project).
func checkout(dir, target, branch string) error {
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", target},
	} {
		if _, err := git(dir, args...); err != nil {
			return errors.Wrap(err, "Failed to prepare the clone")
		}
	}
	if _, err := git(dir, "ls-remote", "--exit-code", "--heads", "origin", branch); err != nil {
		// No such branch upstream: exit status 2, anything else is a real error
		if !strings.Contains(err.Error(), "exit status 2") {
			return errors.Wrap(err, "Failed to reach the target")
		}
		_, err := git(dir, "checkout", "--quiet", "--orphan", branch)
		return errors.Wrap(err, "Failed to start the branch")
	}
	if _, err := git(dir, "fetch", "--quiet", "--depth", "1", "origin", branch); err != nil {
		return errors.Wrap(err, "Failed to fetch the target")
	}
	if _, err := git(dir, "checkout", "--quiet", "-b", branch, "FETCH_HEAD"); err != nil {
		return errors.Wrap(err, "Failed to check out the target")
	}
	return nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
