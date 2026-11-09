package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bigredeye/notmanytask/internal/config"
	"go.uber.org/zap"
)

func forkSyncClient(t *testing.T, handler http.HandlerFunc) *ForksSyncer {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	conf := &config.Config{}
	conf.GitLab.BaseURL = server.URL + "/gitlab"
	conf.GitLab.Api.Token = "test-token"
	conf.GitLab.Group.ID = 9
	conf.GitLab.TemplateProject = "course/template"
	client, err := NewClient(conf, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	syncer := NewForksSyncer(client)
	syncer.pace = 0
	return syncer
}

func TestForkSyncEligibilityAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, project, mutation string
		wantMutation, wantError bool
	}{
		{"behind", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":false,"isSyncing":false}}`, `{"errors":[],"details":{"isSyncing":true}}`, true, false},
		{"caught up", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":0,"hasConflicts":false,"isSyncing":false}}`, "", false, false},
		{"busy", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":false,"isSyncing":true}}`, "", false, false},
		{"conflict", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":true,"isSyncing":false}}`, "", false, false},
		{"different template", `{"forkedFrom":{"id":"gid://gitlab/Project/8"},"forkDetails":{"behind":2,"hasConflicts":false,"isSyncing":false}}`, "", false, false},
		{"not a fork", `{"forkedFrom":null}`, "", false, false},
		{"missing project", `null`, "", false, true},
		{"missing comparison", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":null}`, "", false, true},
		{"unknown conflict", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":null,"isSyncing":false}}`, "", false, true},
		{"unknown behind", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"hasConflicts":false,"isSyncing":false}}`, "", false, true},
		{"unknown syncing", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":false}}`, "", false, true},
		{"mutation rejected", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":false,"isSyncing":false}}`, `{"errors":["not allowed"]}`, true, true},
		{"missing mutation result", `{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":false,"isSyncing":false}}`, `null`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := false
			syncer := forkSyncClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/gitlab/api/graphql" || r.Method != http.MethodPost || r.Header.Get("PRIVATE-TOKEN") != "test-token" {
					t.Errorf("bad GraphQL request: %s %s", r.Method, r.URL)
				}
				var request struct {
					Query     string
					Variables map[string]string
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Variables["path"] != "course/student" || request.Variables["branch"] != "main" {
					t.Errorf("wrong target: %v", request.Variables)
				}
				if strings.HasPrefix(request.Query, "mutation") {
					mutated = true
					fmt.Fprintf(w, `{"data":{"projectSyncFork":%s}}`, tc.mutation)
				} else {
					fmt.Fprintf(w, `{"data":{"project":%s}}`, tc.project)
				}
			})
			started, err := syncer.syncProject(context.Background(), "course/student", 7, "main")
			if mutated != tc.wantMutation || (err != nil) != tc.wantError || started != (tc.wantMutation && !tc.wantError) {
				t.Fatalf("mutated=%v started=%v err=%v", mutated, started, err)
			}
		})
	}
}

func TestForkSyncGraphQLFailuresNeverMutate(t *testing.T) {
	for name, body := range map[string]string{
		"partial data with error": `{"data":{"project":{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":2,"hasConflicts":false,"isSyncing":false}}},"errors":[{"message":"timeout"}]}`,
		"null data":               `{"data":null}`,
		"malformed":               `<html>Bad gateway</html>`,
		"http failure":            ``,
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			syncer := forkSyncClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if body == "" {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				fmt.Fprint(w, body)
			})
			if started, err := syncer.syncProject(context.Background(), "course/student", 7, "main"); err == nil || started || calls != 1 {
				t.Fatalf("started=%v err=%v calls=%d", started, err, calls)
			}
		})
	}
}

func TestForkSyncSweepScopesPaginatesAndRetries(t *testing.T) {
	pages, attempts, successes := 0, 0, 0
	syncer := forkSyncClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gitlab/api/v4/projects/course/template":
			fmt.Fprint(w, `{"id":7,"default_branch":"master"}`)
		case "/gitlab/api/v4/groups/9/projects":
			pages++
			q := r.URL.Query()
			if q.Get("simple") != "true" || q.Get("per_page") != "100" || q.Get("with_shared") != "false" || q.Get("include_subgroups") != "false" || q.Get("archived") != "false" {
				t.Errorf("unscoped list: %s", r.URL)
			}
			if q.Get("page") == "1" {
				w.Header().Set("X-Next-Page", "2")
				fmt.Fprint(w, `[
 {"id":7,"path_with_namespace":"course/template","namespace":{"id":9}},
 {"id":10,"path_with_namespace":"shared/student","namespace":{"id":10}},
 {"id":11,"path_with_namespace":"course/archived","namespace":{"id":9},"archived":true},
 {"id":12,"path_with_namespace":"course/broken","namespace":{"id":9}}]`)
			} else {
				fmt.Fprint(w, `[{"id":13,"path_with_namespace":"course/healthy","namespace":{"id":9}}]`)
			}
		case "/gitlab/api/graphql":
			var request struct {
				Query     string
				Variables map[string]string
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request.Variables["branch"] != "main" {
				t.Errorf("must honor MR target, got %v", request.Variables)
			}
			path := request.Variables["path"]
			if path != "course/broken" && path != "course/healthy" {
				t.Errorf("unexpected project %q", path)
			}
			if strings.HasPrefix(request.Query, "mutation") {
				successes++
				fmt.Fprint(w, `{"data":{"projectSyncFork":{"errors":[],"details":{"isSyncing":true}}}}`)
			} else if path == "course/broken" && attempts == 0 {
				attempts++
				w.WriteHeader(http.StatusBadGateway)
			} else {
				// The healthy project is caught up on the second sweep.
				behind := 1
				if path == "course/healthy" && pages > 2 {
					behind = 0
				}
				fmt.Fprintf(w, `{"data":{"project":{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":%d,"hasConflicts":false,"isSyncing":false}}}}`, behind)
			}
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
		}
	})
	syncer.config.GitLab.MergeRequests = &config.MergeRequestsConfig{TargetBranch: "main"}
	for sweep := 0; sweep < 2; sweep++ {
		if err := syncer.iteration(context.Background()); err != nil {
			t.Fatal(err)
		}
		if successes != sweep+1 {
			t.Fatalf("sweep %d: successes=%d", sweep, successes)
		}
	}
	if pages != 4 {
		t.Fatalf("pages=%d", pages)
	}
}

func TestForkSyncDisabledAndCancellation(t *testing.T) {
	var calls atomic.Int32
	syncer := forkSyncClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"data":{"project":{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":1,"hasConflicts":false,"isSyncing":false}}}}`)
	})
	syncer.Run(context.Background())
	if calls.Load() != 0 {
		t.Fatal("disabled worker made requests")
	}
	// Cancellation interrupts the pacing wait before a mutation.
	syncer.pace = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started, err := syncer.syncProject(ctx, "course/student", 7, "main")
	if err == nil || started || calls.Load() > 1 {
		t.Fatalf("started=%v err=%v calls=%d", started, err, calls.Load())
	}
}

func TestForkSyncDoesNotFollowRedirects(t *testing.T) {
	var forwarded atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Store(true) }))
	defer target.Close()
	syncer := forkSyncClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	_, err := syncer.syncProject(context.Background(), "course/student", 7, "main")
	if err == nil || forwarded.Load() {
		t.Fatalf("err=%v forwarded=%v", err, forwarded.Load())
	}
}

func TestForkSyncPipelineModeAndShutdown(t *testing.T) {
	requested := make(chan struct{})
	syncer := forkSyncClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gitlab/api/v4/projects/course/template":
			fmt.Fprint(w, `{"id":7,"default_branch":"master"}`)
		case "/gitlab/api/v4/groups/9/projects":
			fmt.Fprint(w, `[{"id":12,"path_with_namespace":"course/student","namespace":{"id":9}}]`)
		case "/gitlab/api/graphql":
			var request struct {
				Query     string
				Variables map[string]string
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request.Variables["branch"] != "master" {
				t.Errorf("pipeline mode must use template default branch: %v", request.Variables)
			}
			if strings.HasPrefix(request.Query, "mutation") {
				fmt.Fprint(w, `{"data":{"projectSyncFork":{"errors":[]}}}`)
				close(requested)
			} else {
				fmt.Fprint(w, `{"data":{"project":{"forkedFrom":{"id":"gid://gitlab/Project/7"},"forkDetails":{"behind":1,"hasConflicts":false,"isSyncing":false}}}}`)
			}
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	})
	interval := time.Hour
	syncer.config.PullIntervals.Forks = &interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		syncer.Run(ctx)
		close(done)
	}()
	select {
	case <-requested:
	case <-time.After(3 * time.Second):
		t.Fatal("no initial sweep")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}
