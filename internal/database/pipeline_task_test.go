package database

import "testing"

func TestParseMergeRequestRef(t *testing.T) {
	for ref, want := range map[string]int{
		"refs/merge-requests/1/head":   1,
		"refs/merge-requests/42/merge": 42,
	} {
		if iid, ok := parseMergeRequestRef(ref); !ok || iid != want {
			t.Errorf("parseMergeRequestRef(%q) = %d, %v; want %d", ref, iid, ok, want)
		}
	}
	for _, ref := range []string{"submits/jit/pipeline", "main", "refs/merge-requests/x/head", "refs/merge-requests/1/tail", ""} {
		if _, ok := parseMergeRequestRef(ref); ok {
			t.Errorf("parseMergeRequestRef(%q) unexpectedly matched", ref)
		}
	}
}
