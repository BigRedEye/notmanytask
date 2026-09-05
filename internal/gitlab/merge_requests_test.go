package gitlab

import (
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
)

func TestReadyToMerge(t *testing.T) {
	deadline := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	before := deadline.Add(-time.Hour)
	after := deadline.Add(time.Hour)

	good := func() *branchMergeRequests {
		return &branchMergeRequests{
			Open: &models.MergeRequest{
				State:                 models.MergeRequestStateOpened,
				MergeStatus:           models.MergeRequestStatusCanBeMerged,
				SHA:                   "deadbeef",
				LastPipelineStatus:    models.PipelineStatusSuccess,
				LastPipelineCreatedAt: before,
			},
			LastNoteCreatedAt: before,
		}
	}
	if !readyToMerge(good(), deadline) {
		t.Fatal("quiet green merge request must be merged")
	}

	cases := map[string]func(*branchMergeRequests){
		"conflict":         func(m *branchMergeRequests) { m.Open.MergeStatus = models.MergeRequestStatusCannotBeMerged },
		"status checking":  func(m *branchMergeRequests) { m.Open.MergeStatus = "checking" },
		"status unknown":   func(m *branchMergeRequests) { m.Open.MergeStatus = "" },
		"no sha":           func(m *branchMergeRequests) { m.Open.SHA = "" },
		"unresolved notes": func(m *branchMergeRequests) { m.HasUnresolvedNotes = true },
		"fresh note":       func(m *branchMergeRequests) { m.LastNoteCreatedAt = after },
		"extra changes":    func(m *branchMergeRequests) { m.Open.ExtraChanges = true },
		"no changes":       func(m *branchMergeRequests) { m.Open.NoChanges = true },
		"pipeline failed":  func(m *branchMergeRequests) { m.Open.LastPipelineStatus = models.PipelineStatusFailed },
		"pipeline running": func(m *branchMergeRequests) { m.Open.LastPipelineStatus = models.PipelineStatusRunning },
		"no pipeline":      func(m *branchMergeRequests) { m.Open.LastPipelineStatus = "" },
		"fresh pipeline":   func(m *branchMergeRequests) { m.Open.LastPipelineCreatedAt = after },
		"zero pipeline time": func(m *branchMergeRequests) {
			// zero time is "before" any deadline: a success without a
			// timestamp must not be merged
			m.Open.LastPipelineCreatedAt = time.Time{}
		},
	}
	for name, mutate := range cases {
		m := good()
		mutate(m)
		if readyToMerge(m, deadline) {
			t.Errorf("%s: must not be merged", name)
		}
	}
}

func TestNeedsMergeRequest(t *testing.T) {
	merged := &models.MergeRequest{State: models.MergeRequestStateMerged, SHA: "aaa"}
	cases := []struct {
		name string
		mrs  *branchMergeRequests
		head string
		want bool
	}{
		{"never merged", &branchMergeRequests{}, "aaa", true},
		{"merged, branch unchanged", &branchMergeRequests{Merged: merged}, "aaa", false},
		{"merged, branch moved", &branchMergeRequests{Merged: merged}, "bbb", true},
		{"merged, unknown branch head", &branchMergeRequests{Merged: merged}, "", false},
		{"merged, unknown merged sha", &branchMergeRequests{Merged: &models.MergeRequest{State: models.MergeRequestStateMerged}}, "bbb", false},
	}
	for _, c := range cases {
		if got := needsMergeRequest(c.mrs, c.head); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNormalizeMergeStatus(t *testing.T) {
	cases := map[[2]string]string{
		{"", "can_be_merged"}:                 "can_be_merged",
		{"", "cannot_be_merged"}:              "cannot_be_merged",
		{"mergeable", "cannot_be_merged"}:     "can_be_merged",
		{"conflict", "can_be_merged"}:         "cannot_be_merged",
		{"need_rebase", ""}:                   "cannot_be_merged",
		{"checking", "can_be_merged"}:         "checking",
		{"ci_still_running", "can_be_merged"}: "ci_still_running",
	}
	for in, want := range cases {
		if got := normalizeMergeStatus(in[0], in[1]); got != want {
			t.Errorf("normalizeMergeStatus(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
