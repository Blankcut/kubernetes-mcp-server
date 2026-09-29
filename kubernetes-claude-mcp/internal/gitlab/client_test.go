package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/upstream"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

func TestAttemptRequestWithQueryParameters(t *testing.T) {
	// Create a test server that verifies the request
	var receivedPath string
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	// Create a client
	cfg := &config.GitLabConfig{
		URL:        server.URL,
		AuthToken:  "test-token",
		APIVersion: "v4",
	}

	fullConfig := &config.Config{
		GitLab: *cfg,
		Claude: config.ClaudeConfig{
			APIKey:      "test-claude-key",
			BaseURL:     "https://api.anthropic.com",
			ModelID:     "claude-sonnet-4-6",
			MaxTokens:   8192,
			Temperature: 0.3,
		},
	}

	credProvider := auth.NewCredentialProvider(fullConfig)

	// Load credentials
	ctx := context.Background()
	if err := credProvider.LoadCredentials(ctx); err != nil {
		t.Fatalf("Failed to load credentials: %v", err)
	}

	client := NewClient(cfg, credProvider, logging.NewLogger())

	// Test with query parameters
	endpoint := "projects?membership=true&order_by=updated_at&sort=desc&per_page=100"
	_, err := client.attemptRequest(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Verify the path is correct
	expectedPath := "/api/v4/projects"
	if receivedPath != expectedPath {
		t.Errorf("Expected path %q, got %q", expectedPath, receivedPath)
	}

	// Verify query parameters are preserved
	expectedQuery := "membership=true&order_by=updated_at&sort=desc&per_page=100"
	if receivedQuery != expectedQuery {
		t.Errorf("Expected query %q, got %q", expectedQuery, receivedQuery)
	}
}

func TestAttemptRequestWithoutQueryParameters(t *testing.T) {
	// Create a test server
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	// Create a client
	cfg := &config.GitLabConfig{
		URL:        server.URL,
		AuthToken:  "test-token",
		APIVersion: "v4",
	}

	fullConfig := &config.Config{
		GitLab: *cfg,
		Claude: config.ClaudeConfig{
			APIKey:      "test-claude-key",
			BaseURL:     "https://api.anthropic.com",
			ModelID:     "claude-sonnet-4-6",
			MaxTokens:   8192,
			Temperature: 0.3,
		},
	}

	credProvider := auth.NewCredentialProvider(fullConfig)

	// Load credentials
	ctx := context.Background()
	if err := credProvider.LoadCredentials(ctx); err != nil {
		t.Fatalf("Failed to load credentials: %v", err)
	}

	client := NewClient(cfg, credProvider, logging.NewLogger())

	// Test without query parameters
	endpoint := "projects/123"
	_, err := client.attemptRequest(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Verify the path is correct
	expectedPath := "/api/v4/projects/123"
	if receivedPath != expectedPath {
		t.Errorf("Expected path %q, got %q", expectedPath, receivedPath)
	}
}

func TestAttemptRequestWithAPIPrefix(t *testing.T) {
	// Create a test server
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	// Create a client
	cfg := &config.GitLabConfig{
		URL:        server.URL,
		AuthToken:  "test-token",
		APIVersion: "v4",
	}

	fullConfig := &config.Config{
		GitLab: *cfg,
		Claude: config.ClaudeConfig{
			APIKey:      "test-claude-key",
			BaseURL:     "https://api.anthropic.com",
			ModelID:     "claude-sonnet-4-6",
			MaxTokens:   8192,
			Temperature: 0.3,
		},
	}

	credProvider := auth.NewCredentialProvider(fullConfig)

	// Load credentials
	ctx := context.Background()
	if err := credProvider.LoadCredentials(ctx); err != nil {
		t.Fatalf("Failed to load credentials: %v", err)
	}

	client := NewClient(cfg, credProvider, logging.NewLogger())

	// Test with /api prefix already in endpoint
	endpoint := "/api/v4/version"
	_, err := client.attemptRequest(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Verify the path is correct (should not double-add /api/v4)
	expectedPath := "/api/v4/version"
	if receivedPath != expectedPath {
		t.Errorf("Expected path %q, got %q", expectedPath, receivedPath)
	}
}

// newRetryTestClient returns a client pointed at handler with a backoff short
// enough for tests; the number of attempts is the production one.
func newRetryTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	t.Setenv("GITLAB_AUTH_TOKEN", "")
	t.Setenv("GITLAB_TOKEN", "")

	cfg := &config.Config{
		GitLab: config.GitLabConfig{URL: server.URL, AuthToken: "test-token", APIVersion: "v4"},
		Claude: config.ClaudeConfig{APIKey: "test-claude-key"},
	}
	credProvider := auth.NewCredentialProvider(cfg)
	if err := credProvider.LoadCredentials(context.Background()); err != nil {
		t.Fatalf("Failed to load credentials: %v", err)
	}

	client := NewClient(&cfg.GitLab, credProvider, logging.NewLogger())
	client.backoff = upstream.Backoff{Attempts: upstream.DefaultBackoff.Attempts, Delay: time.Millisecond}
	return client
}

// The GitLab client copied the ArgoCD retry loop and its bug: every error
// status was retried, so a missing file cost three requests and 3s.
func TestDoRequestRetryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		status    int // returned by the first request; later ones succeed
		wantHits  int32
		wantError bool
	}{
		{name: "GET 401 is not retried", method: http.MethodGet, status: http.StatusUnauthorized, wantHits: 1, wantError: true},
		{name: "GET 403 is not retried", method: http.MethodGet, status: http.StatusForbidden, wantHits: 1, wantError: true},
		{name: "GET 404 is not retried", method: http.MethodGet, status: http.StatusNotFound, wantHits: 1, wantError: true},
		{name: "GET 503 is retried", method: http.MethodGet, status: http.StatusServiceUnavailable, wantHits: 2},
		{name: "GET 429 is retried", method: http.MethodGet, status: http.StatusTooManyRequests, wantHits: 2},
		// GitLab may have created the object before failing.
		{name: "POST 502 is not retried", method: http.MethodPost, status: http.StatusBadGateway, wantHits: 1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			client := newRetryTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if hits.Add(1) == 1 {
					http.Error(w, http.StatusText(tt.status), tt.status)
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))

			resp, err := client.doRequest(context.Background(), tt.method, "projects/123", nil)
			if err == nil {
				_ = resp.Body.Close()
			}

			if tt.wantError && upstream.StatusCode(err) != tt.status {
				t.Errorf("err = %v, want status %d", err, tt.status)
			}
			if !tt.wantError && err != nil {
				t.Errorf("err = %v, want success after a retry", err)
			}
			if n := hits.Load(); n != tt.wantHits {
				t.Errorf("GitLab received %d requests, want %d", n, tt.wantHits)
			}
		})
	}
}

// A rate-limited POST is retried, and the retry must carry the same body: the
// request reader is spent by the first attempt.
func TestRetriedPostResendsItsBody(t *testing.T) {
	var hits atomic.Int32
	var gotBody string
	client := newRetryTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBody = body.Body
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"id":1,"body":%q}`, body.Body)
	}))

	if _, err := client.CreateMergeRequestComment(context.Background(), "123", 7, "deploy looks healthy"); err != nil {
		t.Fatalf("CreateMergeRequestComment: %v", err)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("GitLab received %d requests, want 2", n)
	}
	if gotBody != "deploy looks healthy" {
		t.Errorf("retried body = %q, want the original comment", gotBody)
	}
}
