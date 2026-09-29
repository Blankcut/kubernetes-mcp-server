package gitlab

import (
	"bytes"
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
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/upstream"
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
	backoff            upstream.Backoff
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
		backoff:            upstream.DefaultBackoff,
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

// doRequest performs an HTTP request to the GitLab API with authentication and
// retry logic. Any status outside 2xx is returned as an *upstream.StatusError,
// and only failures upstream.Retryable accepts are retried.
func (c *Client) doRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	// A reader is spent by the first attempt; buffer it so that a retry sends
	// the same payload rather than an empty one.
	var payload []byte
	if body != nil {
		var err error
		if payload, err = io.ReadAll(body); err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	return c.backoff.Do(ctx, method,
		func() (*http.Response, error) {
			return c.attemptRequest(ctx, method, endpoint, payload)
		},
		func(attempt int, delay time.Duration, err error) {
			c.logger.Debug("Retrying GitLab request", "attempt", attempt, "delay", delay, "error", err)
		})
}

// attemptRequest makes a single request attempt
func (c *Client) attemptRequest(ctx context.Context, method, endpoint string, payload []byte) (*http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid GitLab URL: %w", err)
	}

	// Parse the endpoint to separate path from query parameters
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	// Add API version if not already in the endpoint path
	endpointPath := endpointURL.Path
	if !strings.HasPrefix(endpointPath, "/api") {
		endpointPath = path.Join("/api", c.config.APIVersion, endpointPath)
	}

	// Set the full path
	u.Path = path.Join(u.Path, endpointPath)

	// Preserve query parameters from the endpoint
	if endpointURL.RawQuery != "" {
		u.RawQuery = endpointURL.RawQuery
	}

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add auth header
	if authErr := c.addAuth(req); authErr != nil {
		return nil, fmt.Errorf("failed to add authentication: %w", authErr)
	}

	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug("Sending request to GitLab API", "method", method, "url", u.String())
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &upstream.TransportError{Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, upstream.NewStatusError("GitLab", resp)
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
