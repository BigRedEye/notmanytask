package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bigredeye/notmanytask/internal/render"
)

func TestWritePublishResult(t *testing.T) {
	result := &render.PublishResult{
		Summary: &render.Summary{Tasks: []string{"a"}, Added: []string{"tasks/a/a.cpp"}},
		Diff:    " tasks/a/a.cpp | 1 +\n",
		Patch:   "diff --git a/tasks/a/a.cpp b/tasks/a/a.cpp\n+int main() {}\n",
	}
	cases := []struct {
		name   string
		opts   render.PublishOptions
		pushed bool
		status string
	}{
		{"dry run", render.PublishOptions{DryRun: true}, false, "dry run, nothing pushed"},
		{"pushed", render.PublishOptions{Target: "git@example:t.git"}, true, "pushed to git@example:t.git"},
		{"nothing", render.PublishOptions{}, false, "nothing to publish"},
	}
	for _, c := range cases {
		result.Pushed = c.pushed
		var out, status bytes.Buffer
		writePublishResult(&out, &status, c.opts, result)
		if out.String() != result.Patch {
			t.Errorf("%s: stdout must be exactly the patch, got %q", c.name, out.String())
		}
		for _, want := range []string{result.Diff, "1 tasks, 1 added", c.status} {
			if !strings.Contains(status.String(), want) {
				t.Errorf("%s: stderr lacks %q: %q", c.name, want, status.String())
			}
		}
	}

	// Without --diff stdout stays empty
	var out, status bytes.Buffer
	writePublishResult(&out, &status, render.PublishOptions{DryRun: true}, &render.PublishResult{Summary: &render.Summary{}})
	if out.Len() != 0 {
		t.Fatalf("stdout must be empty without a patch, got %q", out.String())
	}
}
