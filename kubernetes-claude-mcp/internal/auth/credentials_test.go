package auth

import (
	"context"
	"testing"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
)

// A deployment that has finished moving to Workload Identity Federation has no
// static key to load. Erroring here crash-loops it before the Claude client is
// ever constructed, which is what happened in production on the first attempt.
func TestLoadClaudeCredentialsAllowsFederationWithoutAPIKey(t *testing.T) {
	t.Setenv("CLAUDE_API_KEY", "")

	cfg := &config.Config{}
	cfg.Claude.Federation = config.ClaudeFederationConfig{
		IdentityTokenFile: "/var/run/secrets/anthropic.com/token",
		FederationRuleID:  "fdrl_test",
		OrganizationID:    "org-test",
		ServiceAccountID:  "svac_test",
	}

	p := NewCredentialProvider(cfg)
	if err := p.loadClaudeCredentials(context.Background()); err != nil {
		t.Fatalf("federation-only config should load without error, got: %v", err)
	}
}

// Without either credential the error must stay, so a genuinely misconfigured
// deployment still fails loudly at startup rather than at first request.
func TestLoadClaudeCredentialsStillFailsWithNeither(t *testing.T) {
	t.Setenv("CLAUDE_API_KEY", "")

	p := NewCredentialProvider(&config.Config{})
	if err := p.loadClaudeCredentials(context.Background()); err == nil {
		t.Fatal("expected an error when neither an API key nor federation is configured")
	}
}

func TestLoadClaudeCredentialsStillPrefersAPIKey(t *testing.T) {
	t.Setenv("CLAUDE_API_KEY", "sk-ant-test")

	p := NewCredentialProvider(&config.Config{})
	if err := p.loadClaudeCredentials(context.Background()); err != nil {
		t.Fatalf("api-key config should load, got: %v", err)
	}
	if c := p.credentials[ServiceClaude]; c == nil || c.APIKey != "sk-ant-test" {
		t.Fatal("expected the API key to be stored for ServiceClaude")
	}
}

// The bug: once an ArgoCD token passed its ExpiresAt, GetCredentials released
// its read lock twice (panic) and then called RefreshCredentials while holding
// the write lock RefreshCredentials takes itself (deadlock). Prod is token-only
// with no ExpiresAt, so it never fired there -- but any username/password
// deployment hit it 24h after its first session.
func TestGetCredentialsRefreshesExpiredWithoutDeadlockOrPanic(t *testing.T) {
	p := NewCredentialProvider(&config.Config{})
	p.credentials[ServiceArgoCD] = &Credentials{
		Username:  "mcp",
		Password:  "test-password",
		Token:     "stale",
		ExpiresAt: time.Now().Add(-time.Minute),
	}

	type result struct {
		creds *Credentials
		err   error
		panic any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panic: r}
			}
		}()
		c, err := p.GetCredentials(ServiceArgoCD)
		done <- result{creds: c, err: err}
	}()

	select {
	case r := <-done:
		if r.panic != nil {
			t.Fatalf("GetCredentials panicked: %v", r.panic)
		}
		if r.err != nil {
			t.Fatalf("GetCredentials returned an error: %v", r.err)
		}
		if r.creds == nil || r.creds.Username != "mcp" {
			t.Fatalf("unexpected credentials: %+v", r.creds)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GetCredentials deadlocked on expired credentials")
	}

	// The provider must still be usable afterwards: a leaked lock would
	// block every later caller.
	unlocked := make(chan struct{})
	go func() {
		p.UpdateArgoToken(context.Background(), "fresh")
		close(unlocked)
	}()
	select {
	case <-unlocked:
	case <-time.After(2 * time.Second):
		t.Fatal("provider lock was left held after the refresh")
	}
}

func TestGetCredentialsUnexpiredAndMissing(t *testing.T) {
	p := NewCredentialProvider(&config.Config{})
	p.credentials[ServiceArgoCD] = &Credentials{Token: "t"}
	if c, err := p.GetCredentials(ServiceArgoCD); err != nil || c.Token != "t" {
		t.Fatalf("GetCredentials = %+v, %v", c, err)
	}
	if _, err := p.GetCredentials(ServiceGitLab); err == nil {
		t.Fatal("expected an error for a service with no credentials")
	}
}
