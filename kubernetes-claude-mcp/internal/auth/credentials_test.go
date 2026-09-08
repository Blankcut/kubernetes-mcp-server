package auth

import (
	"context"
	"testing"

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
