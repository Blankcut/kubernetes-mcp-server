package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// FederationConfig configures Workload Identity Federation against the Claude
// API. When it is not Enabled(), the client authenticates with a static API key
// exactly as before -- federation is strictly opt-in, so existing deployments
// and every downstream user of this project are unaffected.
//
// Under federation the workload presents a short-lived OIDC token minted by its
// own identity provider (on Kubernetes, a projected service-account token) and
// exchanges it for an Anthropic access token. There is no long-lived secret to
// store, rotate or leak.
type FederationConfig struct {
	// IdentityTokenFile is the path the identity provider writes the JWT to.
	// On EKS this is a projected serviceAccountToken volume whose audience must
	// be https://api.anthropic.com -- the IRSA token at
	// AWS_WEB_IDENTITY_TOKEN_FILE carries aud: sts.amazonaws.com and is rejected.
	IdentityTokenFile string `yaml:"identityTokenFile"`
	FederationRuleID  string `yaml:"federationRuleID"`
	OrganizationID    string `yaml:"organizationID"`
	ServiceAccountID  string `yaml:"serviceAccountID"`
	// WorkspaceID is required only when the federation rule spans more than one
	// workspace; single-workspace rules may leave it empty.
	WorkspaceID string `yaml:"workspaceID"`
}

// Enabled reports whether enough is configured to attempt federation. It
// deliberately mirrors what the exchange endpoint actually requires, so a
// half-configured deployment falls back to the API key rather than failing.
func (f FederationConfig) Enabled() bool {
	return f.IdentityTokenFile != "" && f.FederationRuleID != "" && f.OrganizationID != ""
}

// tokenResponse is the RFC 6749 §5.1 shape returned by the exchange endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// refreshSkew is how far ahead of expiry a cached token is renewed. It matches
// the advisory-refresh margin the official SDKs use.
const refreshSkew = 120 * time.Second

// federationTokenSource exchanges identity tokens for Anthropic access tokens
// and caches the result until shortly before it expires.
//
// The identity token is re-read from disk on every exchange, never cached.
// Identity tokens carrying a jti claim are single-use: presenting the same one
// twice is rejected. Kubernetes rotates a projected token at roughly 80% of its
// lifetime, so a projection that is long-lived relative to the minted token
// makes every refresh after the first replay a spent jti. Keep
// expirationSeconds well below the minted token's lifetime.
type federationTokenSource struct {
	cfg        FederationConfig
	baseURL    string
	httpClient *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newFederationTokenSource(cfg FederationConfig, baseURL string, httpClient *http.Client) *federationTokenSource {
	return &federationTokenSource{cfg: cfg, baseURL: baseURL, httpClient: httpClient}
}

// Token returns a valid access token, exchanging a fresh identity token when the
// cached one is absent or close to expiry.
func (s *federationTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && time.Now().Before(s.expiresAt.Add(-refreshSkew)) {
		return s.token, nil
	}

	assertion, err := os.ReadFile(s.cfg.IdentityTokenFile) //nolint:gosec // path is operator-supplied config, not untrusted input
	if err != nil {
		return "", fmt.Errorf("failed to read identity token: %w", err)
	}

	payload := map[string]string{
		"grant_type":         "urn:ietf:params:oauth:grant-type:jwt-bearer",
		"assertion":          strings.TrimSpace(string(assertion)),
		"federation_rule_id": s.cfg.FederationRuleID,
		"organization_id":    s.cfg.OrganizationID,
	}
	if s.cfg.ServiceAccountID != "" {
		payload["service_account_id"] = s.cfg.ServiceAccountID
	}
	if s.cfg.WorkspaceID != "" {
		payload["workspace_id"] = s.cfg.WorkspaceID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token exchange request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/oauth/token", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// The endpoint returns an opaque 401 on any match failure. Naming the
		// usual causes here saves a long hunt through an unhelpful message.
		return "", fmt.Errorf(
			"token exchange failed with status %d: %s "+
				"(check the federation rule matches the token's subject and audience, "+
				"and that the identity token is fresh -- tokens are single-use)",
			resp.StatusCode, respBody)
	}

	var tr tokenResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", fmt.Errorf("failed to unmarshal token exchange response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("token exchange returned no access token")
	}

	s.token = tr.AccessToken
	s.expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)

	return s.token, nil
}
