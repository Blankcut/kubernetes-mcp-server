package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
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
