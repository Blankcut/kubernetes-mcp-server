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
	"strings"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
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
}

// NewClient creates a new ArgoCD API client
func NewClient(cfg *config.ArgoCDConfig, credProvider *auth.CredentialProvider, logger *logging.Logger) *Client {
	if logger == nil {
		logger = logging.NewLogger().Named("argocd")
	}

	// Create transport with optional insecure mode
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
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
	defer resp.Body.Close()

	var version struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return fmt.Errorf("failed to decode ArgoCD version: %w", err)
	}

	c.logger.Debug("ArgoCD connectivity check successful", "version", version.Version)
	return nil
}

// doRequest performs an HTTP request to the ArgoCD API with authentication
func (c *Client) doRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
    // Try the request with current credentials
    resp, err := c.attemptRequest(ctx, method, endpoint, body)
    
    // If we get a 401 unauthorized, try to refresh the token and retry once
    if err != nil && resp != nil && resp.StatusCode == http.StatusUnauthorized {
        c.logger.Debug("Received 401 from ArgoCD, attempting to refresh token")
        
        // Only try to refresh the token if we have username/password
        creds, err := c.credentialProvider.GetCredentials(auth.ServiceArgoCD)
        if err == nil && creds.Username != "" && creds.Password != "" {
            // Attempt to create a new session
            newToken, _, err := c.createSession(ctx, creds.Username, creds.Password)
            if err != nil {
                return nil, fmt.Errorf("failed to refresh ArgoCD token: %w", err)
            }
            
            // Update the credentials with the new token
            c.credentialProvider.UpdateArgoToken(ctx, newToken)
            
            // Retry the request with the new token
            return c.attemptRequest(ctx, method, endpoint, body)
        }
    }
    
    return resp, err
}

// attemptRequest makes a single request attempt
func (c *Client) attemptRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
    // This contains the original doRequest logic
    u, err := url.Parse(c.baseURL)
    if err != nil {
        return nil, fmt.Errorf("invalid ArgoCD URL: %w", err)
    }
    u.Path = path.Join(u.Path, endpoint)

    req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    if err := c.addAuth(req); err != nil {
        return nil, fmt.Errorf("failed to add authentication: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")

    c.logger.Debug("Sending request to ArgoCD API", "method", method, "endpoint", endpoint)
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }

    if resp.StatusCode >= 400 && resp.StatusCode != 401 {
        defer resp.Body.Close()
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("ArgoCD API error (status %d): %s", resp.StatusCode, string(body))
    }

    return resp, nil
}

// createSession creates a new ArgoCD session
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
    sessionReqBody, err := json.Marshal(sessionReq)
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
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
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

// refreshToken gets a new token using username/password credentials
func (c *Client) refreshToken(ctx context.Context) (string, time.Time, error) {
	creds, err := c.credentialProvider.GetCredentials(auth.ServiceArgoCD)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get ArgoCD credentials: %w", err)
	}

	if creds.Username == "" || creds.Password == "" {
		return "", time.Time{}, fmt.Errorf("username/password required for token refresh")
	}

	// Create session request
	sessionReq := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		Username: creds.Username,
		Password: creds.Password,
	}

	// Convert to JSON
	sessionReqBody, err := json.Marshal(sessionReq)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to marshal session request: %w", err)
	}

	// Create a new HTTP client without authentication for this request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/v1/session", c.baseURL),
		io.NopCloser(strings.NewReader(string(sessionReqBody))),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create session request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("session request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("failed to create session (status %d): %s", resp.StatusCode, string(body))
	}

	var sessionResp struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode session response: %w", err)
	}

	// ArgoCD tokens typically expire after 24 hours
	expiry := time.Now().Add(24 * time.Hour)

	return sessionResp.Token, expiry, nil
}
