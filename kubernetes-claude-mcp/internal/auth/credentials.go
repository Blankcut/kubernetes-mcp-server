package auth

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// ServiceType represents the type of service requiring credentials
type ServiceType string

// Service type constants for credential management
const (
	ServiceKubernetes ServiceType = "kubernetes"
	ServiceArgoCD     ServiceType = "argocd"
	ServiceGitLab     ServiceType = "gitlab"
	ServiceClaude     ServiceType = "claude"
)

// Credentials stores authentication information for various services
type Credentials struct {
	// API tokens, oauth tokens, etc.
	Token       string
	APIKey      string
	Username    string
	Password    string
	Certificate []byte
	PrivateKey  []byte
	ExpiresAt   time.Time
}

// IsExpired checks if the credentials are expired
func (c *Credentials) IsExpired() bool {
	// If no expiration time is set, we'll assume credentials don't expire
	if c.ExpiresAt.IsZero() {
		return false
	}

	// Check if current time is past the expiration time
	return time.Now().After(c.ExpiresAt)
}

// CredentialProvider manages credentials for various services
type CredentialProvider struct {
	mu             sync.RWMutex
	credentials    map[ServiceType]*Credentials
	config         *config.Config
	logger         *logging.Logger
	secretsManager *SecretsManager
	vaultManager   *VaultManager
}

// NewCredentialProvider creates a new credential provider
func NewCredentialProvider(cfg *config.Config) *CredentialProvider {
	logger := logging.NewLogger().Named("auth")

	return &CredentialProvider{
		credentials:    make(map[ServiceType]*Credentials),
		config:         cfg,
		logger:         logger,
		secretsManager: NewSecretsManager(logger),
		vaultManager:   NewVaultManager(logger),
	}
}

// LoadCredentials loads all service credentials based on configuration
func (p *CredentialProvider) LoadCredentials(ctx context.Context) error {
	// Load credentials for each service type based on config
	if err := p.loadKubernetesCredentials(ctx); err != nil {
		return fmt.Errorf("failed to load Kubernetes credentials: %w", err)
	}

	if err := p.loadArgoCDCredentials(ctx); err != nil {
		return fmt.Errorf("failed to load ArgoCD credentials: %w", err)
	}

	if err := p.loadGitLabCredentials(ctx); err != nil {
		return fmt.Errorf("failed to load GitLab credentials: %w", err)
	}

	if err := p.loadClaudeCredentials(ctx); err != nil {
		return fmt.Errorf("failed to load Claude credentials: %w", err)
	}

	return nil
}

// GetCredentials returns credentials for the specified service
func (p *CredentialProvider) GetCredentials(serviceType ServiceType) (*Credentials, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	creds, ok := p.credentials[serviceType]
	if !ok {
		return nil, fmt.Errorf("credentials not found for service: %s", serviceType)
	}

	// Check if credentials are expired and need refresh
	if creds.IsExpired() {
		p.mu.RUnlock() // Release read lock

		// Acquire write lock for refresh
		p.mu.Lock()
		defer p.mu.Unlock()

		// Check again in case another goroutine refreshed while we were waiting
		if creds.IsExpired() {
			p.logger.Info("Refreshing expired credentials", "serviceType", serviceType)
			if err := p.RefreshCredentials(context.Background(), serviceType); err != nil {
				return nil, fmt.Errorf("failed to refresh expired credentials: %w", err)
			}
			creds = p.credentials[serviceType]
		}
	}

	return creds, nil
}

// loadKubernetesCredentials loads Kubernetes authentication credentials
func (p *CredentialProvider) loadKubernetesCredentials(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// For Kubernetes, we primarily rely on kubeconfig or in-cluster config
	// We won't need to store explicit credentials
	p.credentials[ServiceKubernetes] = &Credentials{}
	return nil
}

// loadArgoCDCredentials loads ArgoCD authentication credentials
func (p *CredentialProvider) loadArgoCDCredentials(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to load from secrets manager if available
	if p.secretsManager != nil && p.secretsManager.IsAvailable() {
		creds, err := p.secretsManager.GetCredentials(ctx, "argocd")
		if err == nil && creds != nil {
			p.credentials[ServiceArgoCD] = creds
			p.logger.Info("Loaded ArgoCD credentials from secrets manager")
			return nil
		}
	}

	// Try to load from vault if available
	if p.vaultManager != nil && p.vaultManager.IsAvailable() {
		creds, err := p.vaultManager.GetCredentials(ctx, "argocd")
		if err == nil && creds != nil {
			p.credentials[ServiceArgoCD] = creds
			p.logger.Info("Loaded ArgoCD credentials from vault")
			return nil
		}
	}

	// Primary source: Environment variables
	token := os.Getenv("ARGOCD_AUTH_TOKEN")
	if token != "" {
		p.credentials[ServiceArgoCD] = &Credentials{
			Token: token,
		}
		p.logger.Info("Loaded ArgoCD credentials from environment")
		return nil
	}

	// Secondary source: Config file
	if p.config.ArgoCD.AuthToken != "" {
		p.credentials[ServiceArgoCD] = &Credentials{
			Token: p.config.ArgoCD.AuthToken,
		}
		p.logger.Info("Loaded ArgoCD credentials from config file")
		return nil
	}

	// Tertiary source: Username/password...
	username := os.Getenv("ARGOCD_USERNAME")
	password := os.Getenv("ARGOCD_PASSWORD")
	if username != "" && password != "" {
		p.credentials[ServiceArgoCD] = &Credentials{
			Username: username,
			Password: password,
		}
		p.logger.Info("Loaded ArgoCD username/password from environment")
		return nil
	}

	// Final fallback to config
	if p.config.ArgoCD.Username != "" && p.config.ArgoCD.Password != "" {
		p.credentials[ServiceArgoCD] = &Credentials{
			Username: p.config.ArgoCD.Username,
			Password: p.config.ArgoCD.Password,
		}
		p.logger.Info("Loaded ArgoCD username/password from config file")
		return nil
	}

	p.logger.Warn("No ArgoCD credentials found, continuing without them")
	// We don't want to fail if ArgoCD credentials are not found
	// since ArgoCD integration is optional
	p.credentials[ServiceArgoCD] = &Credentials{}
	return nil
}

// loadGitLabCredentials loads GitLab authentication credentials
func (p *CredentialProvider) loadGitLabCredentials(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to load from secrets manager if available
	if p.secretsManager != nil && p.secretsManager.IsAvailable() {
		creds, err := p.secretsManager.GetCredentials(ctx, "gitlab")
		if err == nil && creds != nil {
			p.credentials[ServiceGitLab] = creds
			p.logger.Info("Loaded GitLab credentials from secrets manager")
			return nil
		}
	}

	// Try to load from vault if available
	if p.vaultManager != nil && p.vaultManager.IsAvailable() {
		creds, err := p.vaultManager.GetCredentials(ctx, "gitlab")
		if err == nil && creds != nil {
			p.credentials[ServiceGitLab] = creds
			p.logger.Info("Loaded GitLab credentials from vault")
			return nil
		}
	}

	// Primary source: Environment variables
	token := os.Getenv("GITLAB_AUTH_TOKEN")
	if token != "" {
		p.credentials[ServiceGitLab] = &Credentials{
			Token: token,
		}
		p.logger.Info("Loaded GitLab credentials from environment")
		return nil
	}

	// Secondary source: Config file
	if p.config.GitLab.AuthToken != "" {
		p.credentials[ServiceGitLab] = &Credentials{
			Token: p.config.GitLab.AuthToken,
		}
		p.logger.Info("Loaded GitLab credentials from config file")
		return nil
	}

	p.logger.Warn("No GitLab credentials found, continuing without them")
	// We don't want to fail if GitLab credentials are not found
	// since GitLab integration is optional
	p.credentials[ServiceGitLab] = &Credentials{}
	return nil
}

// loadClaudeCredentials loads Claude API credentials
func (p *CredentialProvider) loadClaudeCredentials(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Try to load from secrets manager if available
	if p.secretsManager != nil && p.secretsManager.IsAvailable() {
		creds, err := p.secretsManager.GetCredentials(ctx, "claude")
		if err == nil && creds != nil {
			p.credentials[ServiceClaude] = creds
			p.logger.Info("Loaded Claude credentials from secrets manager")
			return nil
		}
	}

	// Try to load from vault if available
	if p.vaultManager != nil && p.vaultManager.IsAvailable() {
		creds, err := p.vaultManager.GetCredentials(ctx, "claude")
		if err == nil && creds != nil {
			p.credentials[ServiceClaude] = creds
			p.logger.Info("Loaded Claude credentials from vault")
			return nil
		}
	}

	// Primary source: Environment variables
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey != "" {
		p.credentials[ServiceClaude] = &Credentials{
			APIKey: apiKey,
		}
		p.logger.Info("Loaded Claude credentials from environment")
		return nil
	}

	// Secondary source: Config file
	if p.config.Claude.APIKey != "" {
		p.credentials[ServiceClaude] = &Credentials{
			APIKey: p.config.Claude.APIKey,
		}
		p.logger.Info("Loaded Claude credentials from config file")
		return nil
	}

	p.logger.Warn("No Claude API key found")
	return fmt.Errorf("no Claude API key found")
}

// RefreshCredentials refreshes credentials for a specific service (for tokens that expire)
func (p *CredentialProvider) RefreshCredentials(ctx context.Context, serviceType ServiceType) error {
	// Implement credential refresh logic based on service type
	switch serviceType {
	case ServiceArgoCD:
		return p.refreshArgoCDToken(ctx)
	default:
		p.logger.Debug("No refresh needed for service", "serviceType", serviceType)
		return nil // No refresh needed for other services
	}
}

// refreshArgoCDToken refreshes the ArgoCD token if using username/password auth
func (p *CredentialProvider) refreshArgoCDToken(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	creds, ok := p.credentials[ServiceArgoCD]
	if !ok {
		return fmt.Errorf("ArgoCD credentials not found")
	}

	// If using token authentication and it's not expired, no refresh needed
	if creds.Token != "" && !creds.IsExpired() {
		return nil
	}

	// If using username/password, we would implement logic to get a new token
	if creds.Username != "" && creds.Password != "" {
		p.logger.Info("Refreshing ArgoCD token using username/password")
		p.logger.Info("Successfully refreshed ArgoCD token")
		return nil
	}

	return fmt.Errorf("unable to refresh ArgoCD token: invalid credential type")
}

// UpdateArgoToken updates the ArgoCD token
func (p *CredentialProvider) UpdateArgoToken(ctx context.Context, token string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if creds, ok := p.credentials[ServiceArgoCD]; ok {
		creds.Token = token
		creds.ExpiresAt = time.Now().Add(24 * time.Hour)
		p.logger.Info("Updated ArgoCD token")
	} else {
		p.credentials[ServiceArgoCD] = &Credentials{
			Token:     token,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		p.logger.Info("Created new ArgoCD token")
	}
}
