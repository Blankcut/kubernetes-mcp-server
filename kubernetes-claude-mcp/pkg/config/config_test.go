package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentVariableExpansion(t *testing.T) {
	// Create a temporary config file with environment variable placeholders
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `server:
  address: ":8080"
  readTimeout: 30
  writeTimeout: 60
  auth:
    apiKey: "${TEST_API_KEY}"

kubernetes:
  kubeconfig: ""
  inCluster: false
  defaultContext: ""
  defaultNamespace: "default"

argocd:
  url: "${TEST_ARGOCD_URL}"
  authToken: "${TEST_ARGOCD_TOKEN}"
  insecure: true

gitlab:
  url: "https://gitlab.com"
  authToken: "${TEST_GITLAB_TOKEN}"
  apiVersion: "v4"

claude:
  apiKey: "${TEST_CLAUDE_KEY}"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-sonnet-4.5-20250514"
  maxTokens: 4096
  temperature: 0.5
`

	// Write the config file
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Set environment variables
	t.Setenv("TEST_API_KEY", "test-api-key-12345")
	t.Setenv("TEST_ARGOCD_URL", "https://argocd.test.com")
	t.Setenv("TEST_ARGOCD_TOKEN", "test-argocd-token-67890")
	t.Setenv("TEST_GITLAB_TOKEN", "test-gitlab-token-abcde")
	t.Setenv("TEST_CLAUDE_KEY", "test-claude-key-fghij")

	// Load the config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment variables were expanded
	if cfg.Server.Auth.APIKey != "test-api-key-12345" {
		t.Errorf("Expected API key to be 'test-api-key-12345', got '%s'", cfg.Server.Auth.APIKey)
	}

	if cfg.ArgoCD.URL != "https://argocd.test.com" {
		t.Errorf("Expected ArgoCD URL to be 'https://argocd.test.com', got '%s'", cfg.ArgoCD.URL)
	}

	if cfg.ArgoCD.AuthToken != "test-argocd-token-67890" {
		t.Errorf("Expected ArgoCD token to be 'test-argocd-token-67890', got '%s'", cfg.ArgoCD.AuthToken)
	}

	if cfg.GitLab.AuthToken != "test-gitlab-token-abcde" {
		t.Errorf("Expected GitLab token to be 'test-gitlab-token-abcde', got '%s'", cfg.GitLab.AuthToken)
	}

	if cfg.Claude.APIKey != "test-claude-key-fghij" {
		t.Errorf("Expected Claude API key to be 'test-claude-key-fghij', got '%s'", cfg.Claude.APIKey)
	}
}

func TestDirectEnvironmentVariableOverride(t *testing.T) {
	// Create a temporary config file without environment variable placeholders
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `server:
  address: ":8080"
  readTimeout: 30
  writeTimeout: 60
  auth:
    apiKey: "config-file-key"

kubernetes:
  kubeconfig: ""
  inCluster: false
  defaultContext: ""
  defaultNamespace: "default"

argocd:
  url: "https://argocd.example.com"
  authToken: "config-file-argocd-token"
  insecure: true

gitlab:
  url: "https://gitlab.com"
  authToken: "config-file-gitlab-token"
  apiVersion: "v4"

claude:
  apiKey: "config-file-claude-key"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-sonnet-4.5-20250514"
  maxTokens: 4096
  temperature: 0.5
`

	// Write the config file
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Set environment variables to override config file values
	t.Setenv("API_KEY", "env-api-key-12345")

	// Load the config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment variable overrides config file
	if cfg.Server.Auth.APIKey != "env-api-key-12345" {
		t.Errorf("Expected API key to be overridden to 'env-api-key-12345', got '%s'", cfg.Server.Auth.APIKey)
	}
}

// validBase returns a Config that passes Validate, so each case below can
// change exactly one thing and attribute the result to that change.
func validBase() *Config {
	cfg := &Config{}
	cfg.Server.Address = ":8080"
	cfg.Claude.APIKey = "sk-ant-test"
	cfg.Claude.BaseURL = "https://api.anthropic.com"
	cfg.Claude.ModelID = "claude-sonnet-4.5-20250514"
	cfg.Claude.MaxTokens = 4096
	cfg.Claude.Temperature = 0.5
	return cfg
}

func fullFederation() ClaudeFederationConfig {
	return ClaudeFederationConfig{
		IdentityTokenFile: "/var/run/secrets/anthropic.com/token",
		FederationRuleID:  "fdrl_test",
		OrganizationID:    "org-test",
		ServiceAccountID:  "svac_test",
	}
}

// The last step of a move to federation is deleting the static key. Validate
// has to accept that, or the deployment crash-loops on a config that works.
func TestValidateAcceptsFederationWithoutAPIKey(t *testing.T) {
	cfg := validBase()
	cfg.Claude.APIKey = ""
	cfg.Claude.Federation = fullFederation()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("federation-only config should validate, got: %v", err)
	}
}

func TestValidateStillAcceptsAPIKeyWithoutFederation(t *testing.T) {
	if err := validBase().Validate(); err != nil {
		t.Fatalf("api-key-only config should validate, got: %v", err)
	}
}

func TestValidateRejectsNeitherCredential(t *testing.T) {
	cfg := validBase()
	cfg.Claude.APIKey = ""

	if err := cfg.Validate(); err == nil {
		t.Fatal("config with neither API key nor federation should fail validation")
	}
}

// A half-configured federation block is not a credential. Accepting it would
// let the server start and then fall back to an empty API key at request time.
func TestValidateRejectsPartialFederationWithoutAPIKey(t *testing.T) {
	for _, missing := range []string{"IdentityTokenFile", "FederationRuleID", "OrganizationID"} {
		t.Run(missing, func(t *testing.T) {
			cfg := validBase()
			cfg.Claude.APIKey = ""
			fed := fullFederation()
			switch missing {
			case "IdentityTokenFile":
				fed.IdentityTokenFile = ""
			case "FederationRuleID":
				fed.FederationRuleID = ""
			case "OrganizationID":
				fed.OrganizationID = ""
			}
			cfg.Claude.Federation = fed

			if err := cfg.Validate(); err == nil {
				t.Fatalf("federation missing %s should fail validation", missing)
			}
		})
	}
}
