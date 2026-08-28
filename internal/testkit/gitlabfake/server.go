// Package gitlabfake provides a deliberately small stateful GitLab HTTP fake
// for component tests that exercise the real go-gitlab client.
package gitlabfake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/xanzy/go-gitlab"
)

const projectsPath = "/api/v4/projects"

var (
	// ErrDialNotAllowed reports an attempted connection anywhere other than the
	// exact listener owned by a Server.
	ErrDialNotAllowed = errors.New("gitlabfake: dial target not allowed")
	// ErrFailurePending reports an attempt to replace an unconsumed injection.
	ErrFailurePending = errors.New("gitlabfake: failure already pending")
)

// Request is a sanitized journal entry. It intentionally has no raw header or
// body field.
type Request struct {
	Method string              `json:"method"`
	Path   string              `json:"path"`
	Query  map[string][]string `json:"query,omitempty"`
	Fields map[string]any      `json:"fields,omitempty"`
}

type project struct {
	ID                   int                    `json:"id"`
	Name                 string                 `json:"name"`
	Path                 string                 `json:"path"`
	DefaultBranch        string                 `json:"default_branch,omitempty"`
	Visibility           gitlab.VisibilityValue `json:"visibility,omitempty"`
	SharedRunnersEnabled bool                   `json:"shared_runners_enabled"`
	CIConfigPath         string                 `json:"ci_config_path,omitempty"`
	Namespace            *projectNamespace      `json:"namespace,omitempty"`
}

type projectNamespace struct {
	ID int `json:"id"`
}

type injectedFailure struct {
	method string
	path   string
	status int
	body   any
}

// Server owns one loopback-only httptest server and the minimal fake state.
type Server struct {
	server       *httptest.Server
	listenerAddr string

	mu       sync.Mutex
	nextID   int
	projects map[int]project
	journal  []Request
	failure  *injectedFailure
}

// New starts a fake server and registers its cleanup with t.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		nextID:   1,
		projects: make(map[int]project),
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	s.listenerAddr = s.server.Listener.Addr().String()
	t.Cleanup(s.server.Close)
	return s
}

// URL returns the fake root URL. go-gitlab appends /api/v4/ itself.
func (s *Server) URL() string {
	return s.server.URL
}

// HTTPClient returns a client whose transport can connect only to this
// server's exact ephemeral listener. Proxy lookup and DNS are never used for
// rejected targets.
func (s *Server) HTTPClient() *http.Client {
	allowed := s.listenerAddr
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != allowed {
				return nil, fmt.Errorf("%w: %s", ErrDialNotAllowed, address)
			}
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, address)
		},
	}
	return &http.Client{Transport: transport}
}

// NewClient constructs the real go-gitlab client over the loopback-locked
// transport. Retries are disabled so injected failures never cause backoff.
func (s *Server) NewClient(token string) (*gitlab.Client, error) {
	return gitlab.NewClient(
		token,
		gitlab.WithBaseURL(s.URL()),
		gitlab.WithHTTPClient(s.HTTPClient()),
		gitlab.WithoutRetries(),
	)
}

// InjectOnce configures one exact-method/path HTTP failure. The body must be
// JSON encodable and is defensively copied before the injection is published.
func (s *Server) InjectOnce(method, path string, status int, body any) error {
	if method == "" || !strings.HasPrefix(path, "/") {
		return errors.New("gitlabfake: injection requires an exact method and absolute path")
	}
	if status < http.StatusBadRequest || status > 599 {
		return errors.New("gitlabfake: injected status must be an HTTP error")
	}
	copied, err := copyJSON(body)
	if err != nil {
		return fmt.Errorf("gitlabfake: copy injected body: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return ErrFailurePending
	}
	s.failure = &injectedFailure{method: method, path: path, status: status, body: copied}
	return nil
}

// Journal returns a deep defensive copy of the sanitized request journal.
func (s *Server) Journal() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	journal := make([]Request, len(s.journal))
	for i := range s.journal {
		journal[i] = cloneRequest(s.journal[i])
	}
	return journal
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	entry := Request{
		Method: r.Method,
		Path:   path,
		Query:  allowlistedQuery(r.URL.Query()),
	}

	var options *gitlab.CreateProjectOptions
	var decodeErr error
	if r.Method == http.MethodPost && path == projectsPath {
		options, decodeErr = decodeCreateOptions(r)
		if decodeErr == nil {
			entry.Fields = safeCreateFields(options)
		}
	}

	failure := s.recordAndTakeFailure(entry)
	if failure != nil {
		writeJSON(w, failure.status, failure.body)
		return
	}
	if decodeErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "400 Bad Request"})
		return
	}

	s.route(w, r.Method, path, options)
}

func (s *Server) route(w http.ResponseWriter, method, path string, options *gitlab.CreateProjectOptions) {
	switch {
	case path == projectsPath:
		if method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "405 Method Not Allowed"})
			return
		}
		s.createProject(w, options)
		return

	case strings.HasPrefix(path, projectsPath+"/"):
		idText := strings.TrimPrefix(path, projectsPath+"/")
		id, err := strconv.Atoi(idText)
		if err != nil || id <= 0 || strings.Contains(idText, "/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "404 Project Not Found"})
			return
		}
		if method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "405 Method Not Allowed"})
			return
		}
		s.getProject(w, id)
		return

	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "404 Not Found"})
	}
}

func (s *Server) createProject(w http.ResponseWriter, options *gitlab.CreateProjectOptions) {
	if options == nil || options.Name == nil || *options.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "name is missing"})
		return
	}

	p := project{Name: *options.Name, Path: *options.Name}
	if options.Path != nil && *options.Path != "" {
		p.Path = *options.Path
	}
	if options.DefaultBranch != nil {
		p.DefaultBranch = *options.DefaultBranch
	}
	if options.Visibility != nil {
		p.Visibility = *options.Visibility
	}
	if options.SharedRunnersEnabled != nil {
		p.SharedRunnersEnabled = *options.SharedRunnersEnabled
	}
	if options.CIConfigPath != nil {
		p.CIConfigPath = *options.CIConfigPath
	}
	if options.NamespaceID != nil {
		p.Namespace = &projectNamespace{ID: *options.NamespaceID}
	}

	s.mu.Lock()
	p.ID = s.nextID
	s.nextID++
	s.projects[p.ID] = cloneProject(p)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) getProject(w http.ResponseWriter, id int) {
	s.mu.Lock()
	p, ok := s.projects[id]
	if ok {
		p = cloneProject(p)
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "404 Project Not Found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) recordAndTakeFailure(entry Request) *injectedFailure {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.journal = append(s.journal, cloneRequest(entry))
	if s.failure == nil || s.failure.method != entry.Method || s.failure.path != entry.Path {
		return nil
	}
	failure := s.failure
	s.failure = nil
	body, err := copyJSON(failure.body)
	if err != nil {
		panic("gitlabfake: stored injected body became invalid: " + err.Error())
	}
	return &injectedFailure{method: failure.method, path: failure.path, status: failure.status, body: body}
}

func decodeCreateOptions(r *http.Request) (*gitlab.CreateProjectOptions, error) {
	decoder := json.NewDecoder(r.Body)
	options := new(gitlab.CreateProjectOptions)
	if err := decoder.Decode(options); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return options, nil
}

func safeCreateFields(options *gitlab.CreateProjectOptions) map[string]any {
	fields := make(map[string]any)
	putString(fields, "name", options.Name)
	putString(fields, "path", options.Path)
	putInt(fields, "namespace_id", options.NamespaceID)
	putString(fields, "default_branch", options.DefaultBranch)
	if options.Visibility != nil {
		fields["visibility"] = string(*options.Visibility)
	}
	putBool(fields, "shared_runners_enabled", options.SharedRunnersEnabled)
	putString(fields, "ci_config_path", options.CIConfigPath)
	return fields
}

func putString(fields map[string]any, name string, value *string) {
	if value != nil {
		fields[name] = *value
	}
}

func putInt(fields map[string]any, name string, value *int) {
	if value != nil {
		fields[name] = *value
	}
}

func putBool(fields map[string]any, name string, value *bool) {
	if value != nil {
		fields[name] = *value
	}
}

func allowlistedQuery(query url.Values) map[string][]string {
	allowed := map[string]bool{
		"license":                true,
		"statistics":             true,
		"with_custom_attributes": true,
	}
	result := make(map[string][]string)
	for key, values := range query {
		if !allowed[key] {
			continue
		}
		for _, value := range values {
			if value == "true" || value == "false" {
				result[key] = append(result[key], value)
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneRequest(request Request) Request {
	clone := Request{Method: request.Method, Path: request.Path}
	if request.Query != nil {
		clone.Query = make(map[string][]string, len(request.Query))
		for key, values := range request.Query {
			clone.Query[key] = append([]string(nil), values...)
		}
	}
	if request.Fields != nil {
		clone.Fields = make(map[string]any, len(request.Fields))
		for key, value := range request.Fields {
			clone.Fields[key] = cloneJSONValue(value)
		}
	}
	return clone
}

func cloneProject(p project) project {
	clone := p
	if p.Namespace != nil {
		namespace := *p.Namespace
		clone.Namespace = &namespace
	}
	return clone
}

func copyJSON(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	var copied any
	if err := decoder.Decode(&copied); err != nil {
		return nil, err
	}
	return copied, nil
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(value))
		for key, item := range value {
			clone[key] = cloneJSONValue(item)
		}
		return clone
	case []any:
		clone := make([]any, len(value))
		for i := range value {
			clone[i] = cloneJSONValue(value[i])
		}
		return clone
	case []string:
		return append([]string(nil), value...)
	default:
		return value
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
