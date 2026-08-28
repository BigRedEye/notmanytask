package gitlabfake

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/xanzy/go-gitlab"
)

func TestRealClientCreateGetProject(t *testing.T) {
	fake := New(t)
	client, err := fake.NewClient("create-get-token-canary")
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}

	created, createResponse, err := client.Projects.CreateProject(&gitlab.CreateProjectOptions{
		Name:                 gitlab.String("safe-project"),
		Path:                 gitlab.String("safe-path"),
		NamespaceID:          gitlab.Int(42),
		DefaultBranch:        gitlab.String("main"),
		Visibility:           gitlab.Visibility(gitlab.PrivateVisibility),
		SharedRunnersEnabled: gitlab.Bool(true),
		CIConfigPath:         gitlab.String("ci/config.yml"),
	})
	if err != nil {
		t.Fatalf("CreateProject returned an error: %v", err)
	}
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("CreateProject status = %d, want %d", createResponse.StatusCode, http.StatusCreated)
	}
	if contentType := createResponse.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("CreateProject Content-Type = %q, want application/json", contentType)
	}
	if createResponse.TotalPages != 0 || createResponse.CurrentPage != 0 || createResponse.NextPage != 0 {
		t.Fatalf("CreateProject invented pagination metadata: %+v", createResponse)
	}
	if created.ID != 1 || created.Name != "safe-project" || created.Path != "safe-path" {
		t.Fatalf("created project = %+v", created)
	}
	if created.Namespace == nil || created.Namespace.ID != 42 {
		t.Fatalf("created namespace = %+v, want ID 42", created.Namespace)
	}

	created.Name = "mutated-client-copy"
	got, getResponse, err := client.Projects.GetProject(1, &gitlab.GetProjectOptions{
		Statistics: gitlab.Bool(true),
	})
	if err != nil {
		t.Fatalf("GetProject returned an error: %v", err)
	}
	if getResponse.StatusCode != http.StatusOK || getResponse.TotalPages != 0 || getResponse.CurrentPage != 0 {
		t.Fatalf("GetProject response = %+v", getResponse)
	}
	if got.Name != "safe-project" || got.Path != "safe-path" || got.DefaultBranch != "main" {
		t.Fatalf("stored project changed: %+v", got)
	}

	journal := fake.Journal()
	if len(journal) != 2 {
		t.Fatalf("journal length = %d, want 2", len(journal))
	}
	if journal[0].Method != http.MethodPost || journal[0].Path != projectsPath {
		t.Fatalf("create journal entry = %+v", journal[0])
	}
	if journal[0].Fields["name"] != "safe-project" || journal[0].Fields["namespace_id"] != 42 {
		t.Fatalf("create safe fields = %#v", journal[0].Fields)
	}
	if journal[1].Method != http.MethodGet || journal[1].Path != projectsPath+"/1" {
		t.Fatalf("get journal entry = %+v", journal[1])
	}
	if got := journal[1].Query["statistics"]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("get query = %#v, want statistics=true", journal[1].Query)
	}

	journal[0].Fields["name"] = "mutated-journal-copy"
	journal[1].Query["statistics"][0] = "mutated-journal-copy"
	again := fake.Journal()
	if again[0].Fields["name"] != "safe-project" || again[1].Query["statistics"][0] != "true" {
		t.Fatalf("Journal did not return a deep copy: %#v", again)
	}
}

func TestUnknownAndInjectedHTTPFailure(t *testing.T) {
	fake := New(t)
	client, err := fake.NewClient("failure-token-canary")
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}

	_, response, err := client.Projects.GetProject(999, &gitlab.GetProjectOptions{})
	if err == nil || response == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown GetProject error = %v, response = %+v", err, response)
	}

	body := map[string]any{"message": "injected unavailable", "details": []any{"safe"}}
	if err := fake.InjectOnce(http.MethodPost, projectsPath, http.StatusServiceUnavailable, body); err != nil {
		t.Fatalf("InjectOnce returned an error: %v", err)
	}
	body["message"] = "mutated after injection"
	_, response, err = client.Projects.GetProject(998, &gitlab.GetProjectOptions{})
	if err == nil || response == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("nonmatching request error = %v, response = %+v", err, response)
	}
	_, response, err = client.Projects.CreateProject(&gitlab.CreateProjectOptions{Name: gitlab.String("injected")})
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("injected CreateProject error = %v, response = %+v", err, response)
	}
	var gitlabError *gitlab.ErrorResponse
	if !errors.As(err, &gitlabError) || !strings.Contains(gitlabError.Message, "injected unavailable") {
		t.Fatalf("injected error = %v, want copied safe message", err)
	}

	created, response, err := client.Projects.CreateProject(&gitlab.CreateProjectOptions{Name: gitlab.String("after-injection")})
	if err != nil || response.StatusCode != http.StatusCreated || created.ID != 1 {
		t.Fatalf("one-shot injection was not consumed: project=%+v response=%+v error=%v", created, response, err)
	}

	request, err := http.NewRequest(http.MethodDelete, fake.URL()+projectsPath+"/1", nil)
	if err != nil {
		t.Fatalf("NewRequest returned an error: %v", err)
	}
	methodResponse, err := fake.HTTPClient().Do(request)
	if err != nil {
		t.Fatalf("DELETE request returned an error: %v", err)
	}
	defer methodResponse.Body.Close()
	if methodResponse.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE status = %d, want %d", methodResponse.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestConcurrentOneShotFailure(t *testing.T) {
	fake := New(t)
	client, err := fake.NewClient("concurrent-token-canary")
	if err != nil {
		t.Fatalf("NewClient returned an error: %v", err)
	}
	if err := fake.InjectOnce(
		http.MethodPost,
		projectsPath,
		http.StatusConflict,
		map[string]string{"message": "one-shot conflict"},
	); err != nil {
		t.Fatalf("InjectOnce returned an error: %v", err)
	}

	const requests = 12
	type result struct {
		id     int
		status int
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, requests)
	var workers sync.WaitGroup
	for i := 0; i < requests; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			project, response, err := client.Projects.CreateProject(&gitlab.CreateProjectOptions{
				Name: gitlab.String(fmt.Sprintf("concurrent-%d", index)),
			})
			item := result{err: err}
			if project != nil {
				item.id = project.ID
			}
			if response != nil {
				item.status = response.StatusCode
			}
			results <- item
		}(i)
	}
	close(start)
	workers.Wait()
	close(results)

	failures := 0
	ids := make(map[int]bool)
	for item := range results {
		switch item.status {
		case http.StatusConflict:
			failures++
			if item.err == nil {
				t.Fatal("injected conflict returned no error")
			}
		case http.StatusCreated:
			if item.err != nil || item.id <= 0 {
				t.Fatalf("successful concurrent create = %+v", item)
			}
			if ids[item.id] {
				t.Fatalf("duplicate project ID %d", item.id)
			}
			ids[item.id] = true
		default:
			t.Fatalf("unexpected concurrent result: %+v", item)
		}
	}
	if failures != 1 || len(ids) != requests-1 {
		t.Fatalf("failures = %d, unique successful IDs = %d; want 1 and %d", failures, len(ids), requests-1)
	}
	if got := len(fake.Journal()); got != requests {
		t.Fatalf("journal length = %d, want %d", got, requests)
	}
}

func TestTransportRejectsNonListenerDial(t *testing.T) {
	fake := New(t)
	httpClient := fake.HTTPClient()

	for _, target := range []string{
		"http://127.0.0.1:1/second-loopback",
		"http://localhost:1/hostname",
		"http://192.0.2.1:80/external",
	} {
		request, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			t.Fatalf("NewRequest(%q) returned an error: %v", target, err)
		}
		_, err = httpClient.Do(request)
		if !errors.Is(err, ErrDialNotAllowed) {
			t.Fatalf("Do(%q) error = %v, want %v", target, err, ErrDialNotAllowed)
		}
	}
}

func TestJournalRedaction(t *testing.T) {
	fake := New(t)
	const (
		authorizationCanary = "authorization-value-canary"
		privateTokenCanary  = "private-token-value-canary"
		jobTokenCanary      = "job-token-value-canary"
		queryTokenCanary    = "query-token-value-canary"
		allowlistCanary     = "allowlisted-query-value-canary"
		importUserCanary    = "import-user-value-canary"
		importPassCanary    = "import-password-value-canary"
		bodyTokenCanary     = "body-token-value-canary"
		bodyPasswordCanary  = "body-password-value-canary"
		bodySecretCanary    = "body-secret-value-canary"
	)

	body := `{
		"name":"visible-safe-name",
		"path":"visible-safe-path",
		"namespace_id":77,
		"import_url":"https://` + importUserCanary + `:` + importPassCanary + `@example.invalid/repo.git",
		"access_token":"` + bodyTokenCanary + `",
		"database_password":"` + bodyPasswordCanary + `",
		"client_secret":"` + bodySecretCanary + `"
	}`
	request, err := http.NewRequest(
		http.MethodPost,
		fake.URL()+projectsPath+"?private_token="+queryTokenCanary+"&statistics="+allowlistCanary+"&license=true",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("NewRequest returned an error: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+authorizationCanary)
	request.Header.Set("Private-Token", privateTokenCanary)
	request.Header.Set("Job-Token", jobTokenCanary)
	request.Header.Set("Content-Type", "application/json")
	response, err := fake.HTTPClient().Do(request)
	if err != nil {
		t.Fatalf("redaction request returned an error: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("redaction request status = %d, want %d", response.StatusCode, http.StatusCreated)
	}

	journal := fake.Journal()
	encoded, err := json.Marshal(journal)
	if err != nil {
		t.Fatalf("Marshal journal returned an error: %v", err)
	}
	serialized := string(encoded)
	for _, forbidden := range []string{
		authorizationCanary,
		privateTokenCanary,
		jobTokenCanary,
		queryTokenCanary,
		allowlistCanary,
		importUserCanary,
		importPassCanary,
		bodyTokenCanary,
		bodyPasswordCanary,
		bodySecretCanary,
		"Authorization",
		"Private-Token",
		"Job-Token",
		"import_url",
		"access_token",
		"database_password",
		"client_secret",
		"private_token",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Errorf("serialized journal contains forbidden value %q: %s", forbidden, serialized)
		}
	}
	if len(journal) != 1 || journal[0].Fields["name"] != "visible-safe-name" || journal[0].Fields["namespace_id"] != 77 {
		t.Fatalf("safe fields missing from journal: %#v", journal)
	}
	if _, ok := journal[0].Query["statistics"]; ok {
		t.Fatalf("non-boolean allowlisted query was retained: %#v", journal[0].Query)
	}
	if got := journal[0].Query["license"]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("safe query missing from journal: %#v", journal[0].Query)
	}
}
