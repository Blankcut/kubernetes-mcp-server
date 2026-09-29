package argocd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/upstream"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// Client handles communication with the ArgoCD API
type Client struct {
	baseURL            string
	httpClient         *http.Client
	credentialProvider *auth.CredentialProvider
	config             *config.ArgoCDConfig
	logger             *logging.Logger
	backoff            upstream.Backoff

	// authz caches the applications:get probe; see authz.go.
	authzMu sync.RWMutex
	authz   AuthzStatus
}

// NewClient creates a new ArgoCD API client
func NewClient(cfg *config.ArgoCDConfig, credProvider *auth.CredentialProvider, logger *logging.Logger) *Client {
	if logger == nil {
		logger = logging.NewLogger().Named("argocd")
	}

	// Create transport with optional insecure mode
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure, //nolint:gosec // Configurable for development environments
		},
	}

	return &Client{
		baseURL: cfg.URL,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		credentialProvider: credProvider,
		config:             cfg,
		logger:             logger,
		backoff:            upstream.DefaultBackoff,
		authz:              AuthzStatus{State: AuthzUnknown},
	}
}

// CheckConnectivity tests the connection to the ArgoCD API
func (c *Client) CheckConnectivity(ctx context.Context) error {
	c.logger.Debug("Checking ArgoCD connectivity")

	// Try to get ArgoCD version as a basic connectivity test
	endpoint := "/api/version"
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to ArgoCD: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var version struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return fmt.Errorf("failed to decode ArgoCD version: %w", err)
	}

	c.logger.Debug("ArgoCD connectivity check successful", "version", version.Version)
	return nil
}

// doRequest performs an HTTP request to the ArgoCD API with authentication and
// retry logic.
//
// Any status outside 2xx is returned as an *upstream.StatusError, so an error
// body is never decoded as a result: a revoked token used to make list and get
// calls quietly return nothing. Transient failures are retried as
// upstream.Retryable describes. A 401 with username/password credentials
// configured creates a new session and retries the request once.
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

	send := func() (*http.Response, error) {
		return c.attemptRequest(ctx, method, endpoint, payload)
	}
	logRetry := func(attempt int, delay time.Duration, err error) {
		c.logger.Debug("Retrying ArgoCD request", "attempt", attempt, "delay", delay, "error", err)
	}

	resp, err := c.backoff.Do(ctx, method, send, logRetry)
	if upstream.StatusCode(err) == http.StatusUnauthorized && c.refreshSession(ctx) {
		resp, err = c.backoff.Do(ctx, method, send, logRetry)
	}
	return resp, err
}

// refreshSession replaces a token ArgoCD rejected with a new session, and
// reports whether there is a new token to retry with. Only username/password
// credentials can be refreshed; a configured API token cannot.
func (c *Client) refreshSession(ctx context.Context) bool {
	creds, err := c.credentialProvider.GetCredentials(auth.ServiceArgoCD)
	if err != nil || creds.Username == "" || creds.Password == "" {
		return false
	}

	c.logger.Debug("ArgoCD rejected the session token, creating a new session")
	token, _, err := c.createSession(ctx, creds.Username, creds.Password)
	if err != nil {
		c.logger.Warn("Failed to refresh ArgoCD session", "error", err)
		return false
	}

	c.credentialProvider.UpdateArgoToken(ctx, token)
	c.logger.Debug("Refreshed ArgoCD session token")
	return true
}

// attemptRequest makes a single request attempt
func (c *Client) attemptRequest(ctx context.Context, method, endpoint string, payload []byte) (*http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ArgoCD URL: %w", err)
	}
	u.Path = path.Join(u.Path, endpoint)

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if authErr := c.addAuth(req); authErr != nil {
		return nil, fmt.Errorf("failed to add authentication: %w", authErr)
	}

	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug("Sending request to ArgoCD API", "method", method, "endpoint", endpoint)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &upstream.TransportError{Err: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, upstream.NewStatusError("ArgoCD", resp)
	}

	return resp, nil
}

// createSession creates a new ArgoCD session
//
//nolint:unparam // time.Time return value reserved for future token expiration tracking
func (c *Client) createSession(ctx context.Context, username, password string) (string, time.Time, error) {
	// Create session request
	sessionReq := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		Username: username,
		Password: password,
	}

	// Convert to JSON
	sessionReqBody, err := json.Marshal(sessionReq) //nolint:gosec // G117: credentials are intentionally sent to the ArgoCD session endpoint to authenticate
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to marshal session request: %w", err)
	}

	// Create a new HTTP client without authentication for this request
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid ArgoCD URL: %w", err)
	}
	u.Path = path.Join(u.Path, "/api/v1/session")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		u.String(),
		bytes.NewReader(sessionReqBody),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create session request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("session request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("failed to create session (status %d): failed to read response body: %w", resp.StatusCode, err)
		}
		return "", time.Time{}, fmt.Errorf("failed to create session (status %d): %s", resp.StatusCode, string(body))
	}

	var sessionResp struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode session response: %w", err)
	}

	// ArgoCD tokens will expire after 24 hours by default...
	expiry := time.Now().Add(24 * time.Hour)

	return sessionResp.Token, expiry, nil
}

// addAuth adds authentication to the request
func (c *Client) addAuth(req *http.Request) error {
	creds, err := c.credentialProvider.GetCredentials(auth.ServiceArgoCD)
	if err != nil {
		return fmt.Errorf("failed to get ArgoCD credentials: %w", err)
	}

	if creds.Token != "" {
		// Set both header formats that ArgoCD might accept
		req.Header.Set("Authorization", "Bearer "+creds.Token)
		req.Header.Set("Cookie", "argocd.token="+creds.Token)
		return nil
	}

	if creds.Username != "" && creds.Password != "" {
		// We need to get a session token first
		token, _, err := c.createSession(req.Context(), creds.Username, creds.Password)
		if err != nil {
			return fmt.Errorf("failed to create ArgoCD session: %w", err)
		}

		// Update credentials with the new token
		c.credentialProvider.UpdateArgoToken(req.Context(), token)

		// Set both header formats
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Cookie", "argocd.token="+token)
		return nil
	}

	return fmt.Errorf("no valid ArgoCD credentials available")
}
