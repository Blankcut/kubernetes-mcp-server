package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// Client handles communication with the GitLab API
type Client struct {
	baseURL            string
	httpClient         *http.Client
	credentialProvider *auth.CredentialProvider
	config             *config.GitLabConfig
	logger             *logging.Logger
}

// NewClient creates a new GitLab API client
func NewClient(cfg *config.GitLabConfig, credProvider *auth.CredentialProvider, logger *logging.Logger) *Client {
	if logger == nil {
		logger = logging.NewLogger().Named("gitlab")
	}

	return &Client{
		baseURL: cfg.URL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		credentialProvider: credProvider,
		config:             cfg,
		logger:             logger,
	}
}

// CheckConnectivity tests the connection to the GitLab API
func (c *Client) CheckConnectivity(ctx context.Context) error {
	c.logger.Debug("Checking GitLab connectivity")

	// Try to get version information
	endpoint := "/api/v4/version"
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to GitLab: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var version struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return fmt.Errorf("failed to decode GitLab version: %w", err)
	}

	c.logger.Debug("GitLab connectivity check successful", "version", version.Version)
	return nil
}

// doRequest performs an HTTP request to the GitLab API with authentication and retry logic
func (c *Client) doRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	const maxRetries = 3
	const baseDelay = 1 * time.Second

	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Try the request
		resp, lastErr = c.attemptRequest(ctx, method, endpoint, body)

		// If successful, return immediately
		if lastErr == nil {
			return resp, nil
		}

		// For errors, check if we should retry
		if attempt < maxRetries-1 && c.shouldRetry(lastErr, resp) {
			delay := time.Duration(1<<uint(attempt)) * baseDelay // Exponential backoff
			c.logger.Debug("Retrying GitLab request", "attempt", attempt+1, "delay", delay, "error", lastErr)

			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// If this was the last attempt or we shouldn't retry, break
		break
	}

	return resp, lastErr
}

// shouldRetry determines if a request should be retried based on the error and response
func (c *Client) shouldRetry(err error, resp *http.Response) bool {
	// Retry on network errors
	if err != nil && resp == nil {
		return true
	}

	// Retry on specific HTTP status codes
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusTooManyRequests, // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout:      // 504
			return true
		}
	}

	return false
}

// attemptRequest makes a single request attempt
func (c *Client) attemptRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid GitLab URL: %w", err)
	}

	// Add API version if not already in the endpoint
	if !strings.HasPrefix(endpoint, "/api") {
		endpoint = path.Join("/api", c.config.APIVersion, endpoint)
	}

	u.Path = path.Join(u.Path, endpoint)

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add auth header
	if err := c.addAuth(req); err != nil {
		return nil, fmt.Errorf("failed to add authentication: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug("Sending request to GitLab API", "method", method, "endpoint", endpoint)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("GitLab API error (status %d): failed to read response body: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// addAuth adds authentication to the request
func (c *Client) addAuth(req *http.Request) error {
	creds, err := c.credentialProvider.GetCredentials(auth.ServiceGitLab)
	if err != nil {
		return fmt.Errorf("failed to get GitLab credentials: %w", err)
	}

	if creds.Token != "" {
		req.Header.Set("PRIVATE-TOKEN", creds.Token)
		return nil
	}

	return fmt.Errorf("no valid GitLab credentials available")
}
