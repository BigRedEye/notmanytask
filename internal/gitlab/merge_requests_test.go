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
