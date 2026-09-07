package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeIdentityToken(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write identity token: %v", err)
	}
	return path
}

func TestFederationConfigEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  FederationConfig
		want bool
	}{
		{"empty is disabled", FederationConfig{}, false},
		{
			"api key only deployments stay disabled",
			FederationConfig{ServiceAccountID: "svac_x", WorkspaceID: "wrkspc_x"},
			false,
		},
		{
			"missing organization is disabled",
			FederationConfig{IdentityTokenFile: "/tmp/token", FederationRuleID: "fdrl_x"},
			false,
		},
		{
			"minimum viable set is enabled",
			FederationConfig{IdentityTokenFile: "/tmp/token", FederationRuleID: "fdrl_x", OrganizationID: "org"},
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFederationTokenSourceExchangesAndCaches(t *testing.T) {
	var exchanges int
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		exchanges++
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "sk-ant-oat01-test",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer srv.Close()

	src := newFederationTokenSource(&FederationConfig{
		IdentityTokenFile: writeIdentityToken(t, "  header.payload.signature\n"),
		FederationRuleID:  "fdrl_test",
		OrganizationID:    "org-test",
		ServiceAccountID:  "svac_test",
		WorkspaceID:       "wrkspc_test",
	}, srv.URL, srv.Client())

	token, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() returned error: %v", err)
	}
	if token != "sk-ant-oat01-test" {
		t.Errorf("token = %q, want sk-ant-oat01-test", token)
	}

	// Surrounding whitespace must be stripped: a trailing newline in the
	// projected file would otherwise be sent as part of the assertion.
	if gotBody["assertion"] != "header.payload.signature" {
		t.Errorf("assertion = %q, want it trimmed", gotBody["assertion"])
	}
	if gotBody["grant_type"] != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		t.Errorf("grant_type = %q", gotBody["grant_type"])
	}
	if gotBody["workspace_id"] != "wrkspc_test" {
		t.Errorf("workspace_id = %q, want wrkspc_test", gotBody["workspace_id"])
	}

	// A second call well inside the token's lifetime must reuse the cached
	// value. Re-exchanging would replay a spent jti and fail.
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("second Token() returned error: %v", err)
	}
	if exchanges != 1 {
		t.Errorf("exchanges = %d, want 1 (second call should hit the cache)", exchanges)
	}
}

func TestFederationTokenSourceRefreshesNearExpiry(t *testing.T) {
	var exchanges int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		exchanges++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "sk-ant-oat01-test", ExpiresIn: 3600})
	}))
	defer srv.Close()

	src := newFederationTokenSource(&FederationConfig{
		IdentityTokenFile: writeIdentityToken(t, "jwt"),
		FederationRuleID:  "fdrl_test",
		OrganizationID:    "org-test",
	}, srv.URL, srv.Client())

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("Token() returned error: %v", err)
	}

	// Pull expiry inside the refresh margin; the next call must re-exchange.
	src.mu.Lock()
	src.expiresAt = time.Now().Add(refreshSkew / 2)
	src.mu.Unlock()

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("Token() after expiry returned error: %v", err)
	}
	if exchanges != 2 {
		t.Errorf("exchanges = %d, want 2 (token near expiry should refresh)", exchanges)
	}
}

func TestFederationTokenSourceSurfacesExchangeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error"}}`))
	}))
	defer srv.Close()

	src := newFederationTokenSource(&FederationConfig{
		IdentityTokenFile: writeIdentityToken(t, "jwt"),
		FederationRuleID:  "fdrl_test",
		OrganizationID:    "org-test",
	}, srv.URL, srv.Client())

	_, err := src.Token(context.Background())
	if err == nil {
		t.Fatal("expected an error on a 401 exchange, got nil")
	}
}

func TestFederationTokenSourceMissingTokenFile(t *testing.T) {
	src := newFederationTokenSource(&FederationConfig{
		IdentityTokenFile: filepath.Join(t.TempDir(), "does-not-exist"),
		FederationRuleID:  "fdrl_test",
		OrganizationID:    "org-test",
	}, "https://api.anthropic.com", http.DefaultClient)

	if _, err := src.Token(context.Background()); err == nil {
		t.Fatal("expected an error when the identity token file is absent, got nil")
	}
}

func TestNewClientOnlyEnablesFederationWhenConfigured(t *testing.T) {
	apiKeyOnly := NewClient(&ClaudeConfig{APIKey: "sk-ant-test", BaseURL: "https://api.anthropic.com"}, nil)
	if apiKeyOnly.federation != nil {
		t.Error("federation should be nil when only an API key is configured")
	}

	federated := NewClient(&ClaudeConfig{
		BaseURL: "https://api.anthropic.com",
		Federation: FederationConfig{
			IdentityTokenFile: "/var/run/secrets/anthropic.com/token",
			FederationRuleID:  "fdrl_test",
			OrganizationID:    "org-test",
		},
	}, nil)
	if federated.federation == nil {
		t.Error("federation should be initialised when configured")
	}
}
