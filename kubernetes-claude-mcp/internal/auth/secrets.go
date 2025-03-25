package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// SecretsManager handles access to secrets stored in various backends
type SecretsManager struct {
	logger *logging.Logger
	// Directory where secrets files are stored
	secretsDir string
	// Flag to indicate if secrets manager is available
	available bool
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager(logger *logging.Logger) *SecretsManager {
	if logger == nil {
		logger = logging.NewLogger().Named("secrets")
	}
	
	// Default secrets directory is ./secrets
	secretsDir := os.Getenv("SECRETS_DIR")
	if secretsDir == "" {
		secretsDir = "./secrets"
	}
	
	// Check if secrets directory exists
	_, err := os.Stat(secretsDir)
	available := err == nil
	
	if !available {
		logger.Warn("Secrets directory not available", "directory", secretsDir)
	}
	
	return &SecretsManager{
		logger:     logger,
		secretsDir: secretsDir,
		available:  available,
	}
}

// IsAvailable returns true if the secrets manager is available
func (sm *SecretsManager) IsAvailable() bool {
	return sm.available
}

// GetCredentials retrieves credentials for a service from the secrets manager
func (sm *SecretsManager) GetCredentials(ctx context.Context, service string) (*Credentials, error) {
	if !sm.available {
		return nil, fmt.Errorf("secrets manager not available")
	}
	
	// Build paths to potential secret files
	tokenPath := filepath.Join(sm.secretsDir, service, "token")
	apiKeyPath := filepath.Join(sm.secretsDir, service, "apikey")
	usernamePath := filepath.Join(sm.secretsDir, service, "username")
	passwordPath := filepath.Join(sm.secretsDir, service, "password")
	
	// Initialize credentials
	creds := &Credentials{}
	
	// Try to read token
	tokenBytes, err := os.ReadFile(tokenPath)
	if err == nil {
		creds.Token = string(tokenBytes)
		sm.logger.Debug("Loaded token from file", "service", service)
	}
	
	// Try to read API key
	apiKeyBytes, err := os.ReadFile(apiKeyPath)
	if err == nil {
		creds.APIKey = string(apiKeyBytes)
		sm.logger.Debug("Loaded API key from file", "service", service)
	}
	
	// Try to read username
	usernameBytes, err := os.ReadFile(usernamePath)
	if err == nil {
		creds.Username = string(usernameBytes)
		sm.logger.Debug("Loaded username from file", "service", service)
	}
	
	// Try to read password
	passwordBytes, err := os.ReadFile(passwordPath)
	if err == nil {
		creds.Password = string(passwordBytes)
		sm.logger.Debug("Loaded password from file", "service", service)
	}
	
	// Check if we loaded any credentials
	if creds.Token == "" && creds.APIKey == "" && creds.Username == "" && creds.Password == "" {
		return nil, fmt.Errorf("no credentials found for service: %s", service)
	}
	
	return creds, nil
}

// SaveCredentials saves credentials for a service to the secrets manager
func (sm *SecretsManager) SaveCredentials(ctx context.Context, service string, creds *Credentials) error {
	if !sm.available {
		return fmt.Errorf("secrets manager not available")
	}
	
	// Create service directory if it doesn't exist
	serviceDir := filepath.Join(sm.secretsDir, service)
	if err := os.MkdirAll(serviceDir, 0700); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}
	
	// Save token if provided
	if creds.Token != "" {
		tokenPath := filepath.Join(serviceDir, "token")
		if err := os.WriteFile(tokenPath, []byte(creds.Token), 0600); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}
		sm.logger.Debug("Saved token to file", "service", service)
	}
	
	// Save API key if provided
	if creds.APIKey != "" {
		apiKeyPath := filepath.Join(serviceDir, "apikey")
		if err := os.WriteFile(apiKeyPath, []byte(creds.APIKey), 0600); err != nil {
			return fmt.Errorf("failed to save API key: %w", err)
		}
		sm.logger.Debug("Saved API key to file", "service", service)
	}
	
	// Save username if provided
	if creds.Username != "" {
		usernamePath := filepath.Join(serviceDir, "username")
		if err := os.WriteFile(usernamePath, []byte(creds.Username), 0600); err != nil {
			return fmt.Errorf("failed to save username: %w", err)
		}
		sm.logger.Debug("Saved username to file", "service", service)
	}
	
	// Save password if provided
	if creds.Password != "" {
		passwordPath := filepath.Join(serviceDir, "password")
		if err := os.WriteFile(passwordPath, []byte(creds.Password), 0600); err != nil {
			return fmt.Errorf("failed to save password: %w", err)
		}
		sm.logger.Debug("Saved password to file", "service", service)
	}
	
	return nil
}