package gitlab

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/bigredeye/notmanytask/internal/config"
)

func TestForkFinished(t *testing.T) {
	cases := map[string][2]bool{ // status -> {ready, failed}
		"finished":  {true, false},
		"none":      {true, false},
		"":          {true, false},
		"scheduled": {false, false},
		"started":   {false, false},
		"failed":    {false, true},
	}
	for status, want := range cases {
		ready, failed := forkFinished(status)
		if ready != want[0] || failed != want[1] {
			t.Errorf("%q: ready=%v failed=%v, want %v", status, ready, failed, want)
		}
	}
}

func TestNewClientResolvesGroupID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/hse-advanced-cpp/2026" {
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"id": 4155, "full_path": "hse-advanced-cpp/2026"}`))
	}))
	defer server.Close()

	conf := &config.Config{}
	conf.GitLab.BaseURL = server.URL
	conf.GitLab.Group.Name = "hse-advanced-cpp/2026"
	if _, err := NewClient(conf, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	if conf.GitLab.Group.ID != 4155 {
		t.Fatalf("group id = %d, want 4155", conf.GitLab.Group.ID)
	}

	// An explicit id is kept without asking gitlab
	conf.GitLab.Group.ID = 7
	conf.GitLab.BaseURL = "http://127.0.0.1:1"
	if _, err := NewClient(conf, zap.NewNop()); err != nil || conf.GitLab.Group.ID != 7 {
		t.Fatalf("explicit id must be kept: %d %v", conf.GitLab.Group.ID, err)
	}
}
