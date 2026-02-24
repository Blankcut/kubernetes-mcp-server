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
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
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
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
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
