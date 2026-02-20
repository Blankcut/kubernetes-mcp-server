package unit

import (
	"testing"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
	"github.com/stretchr/testify/assert"
)

func TestLogging_NewLogger(t *testing.T) {
	logger := logging.NewLogger()
	assert.NotNil(t, logger)

	// Test that we can log without errors
	logger.Info("test message")
	logger.Debug("debug message")
	logger.Warn("warning message")
	logger.Error("error message")
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "localhost:8080",
		},
		Claude: config.ClaudeConfig{
			APIKey:      "sk-test-key",
			BaseURL:     "https://api.anthropic.com",
			ModelID:     "claude-4-sonnet-20250522",
			MaxTokens:   8192,
			Temperature: 0.3,
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_MissingAddress(t *testing.T) {
	cfg := &config.Config{
		Claude: config.ClaudeConfig{
			APIKey:  "sk-test-key",
			ModelID: "claude-4-sonnet-20250522",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server address is required")
}

func TestConfig_Validate_MissingClaudeAPIKey(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "localhost:8080",
		},
		Claude: config.ClaudeConfig{
			ModelID: "claude-4-sonnet-20250522",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Claude API key is required")
}

func TestConfig_Validate_MissingClaudeModelID(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "localhost:8080",
		},
		Claude: config.ClaudeConfig{
			APIKey: "sk-test-key",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Claude model ID is required")
}

func TestConfig_Creation(t *testing.T) {
	// Test that we can create config structs
	argoCfg := &config.ArgoCDConfig{
		URL:       "https://argocd.example.com",
		AuthToken: "test-token",
	}
	assert.NotNil(t, argoCfg)
	assert.Equal(t, "https://argocd.example.com", argoCfg.URL)

	gitlabCfg := &config.GitLabConfig{
		URL:        "https://gitlab.example.com",
		AuthToken:  "test-token",
		APIVersion: "v4",
	}
	assert.NotNil(t, gitlabCfg)
	assert.Equal(t, "https://gitlab.example.com", gitlabCfg.URL)

	claudeCfg := &config.ClaudeConfig{
		APIKey:      "sk-test-key",
		BaseURL:     "https://api.anthropic.com",
		ModelID:     "claude-4-sonnet-20250522",
		MaxTokens:   8192,
		Temperature: 0.3,
	}
	assert.NotNil(t, claudeCfg)
	assert.Equal(t, "claude-4-sonnet-20250522", claudeCfg.ModelID)
}

// Test basic functionality without external dependencies
func TestBasicFunctionality(t *testing.T) {
	// Test that the test framework itself works
	assert.True(t, true)
	assert.False(t, false)
	assert.Equal(t, "test", "test")
	assert.NotEqual(t, "test", "different")
}
