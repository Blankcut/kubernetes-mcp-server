package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config holds all configuration for the MCP server
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Kubernetes KubernetesConfig `yaml:"kubernetes"`
	ArgoCD     ArgoCDConfig     `yaml:"argocd"`
	GitLab     GitLabConfig     `yaml:"gitlab"`
	Claude     ClaudeConfig     `yaml:"claude"`
}

// ServerConfig holds the HTTP server configuration
type ServerConfig struct {
	Address      string `yaml:"address"`
	ReadTimeout  int    `yaml:"readTimeout"`
	WriteTimeout int    `yaml:"writeTimeout"`
	Auth         struct {
		APIKey string `yaml:"apiKey"`
	} `yaml:"auth"`
}

// KubernetesConfig holds configuration for Kubernetes client
type KubernetesConfig struct {
	KubeConfig       string `yaml:"kubeconfig"`
	InCluster        bool   `yaml:"inCluster"`
	DefaultContext   string `yaml:"defaultContext"`
	DefaultNamespace string `yaml:"defaultNamespace"`
}

// ArgoCDConfig holds configuration for the ArgoCD client
type ArgoCDConfig struct {
	URL       string `yaml:"url"`
	AuthToken string `yaml:"authToken"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Insecure  bool   `yaml:"insecure"`
}

// GitLabConfig holds configuration for the GitLab client
type GitLabConfig struct {
	URL        string `yaml:"url"`
	AuthToken  string `yaml:"authToken"`
	APIVersion string `yaml:"apiVersion"`
}

// ClaudeConfig holds configuration for the Claude API client
type ClaudeConfig struct {
	APIKey      string  `yaml:"apiKey"`
	BaseURL     string  `yaml:"baseURL"`
	ModelID     string  `yaml:"modelID"`
	MaxTokens   int     `yaml:"maxTokens"`
	Temperature float64 `yaml:"temperature"`
	// Federation is optional. Leave it unset to authenticate with APIKey.
	Federation ClaudeFederationConfig `yaml:"federation"`
}

// ClaudeFederationConfig configures Workload Identity Federation, an optional
// alternative to a static API key. When unset the client keeps using APIKey, so
// existing deployments need no changes.
type ClaudeFederationConfig struct {
	IdentityTokenFile string `yaml:"identityTokenFile"`
	FederationRuleID  string `yaml:"federationRuleID"`
	OrganizationID    string `yaml:"organizationID"`
	ServiceAccountID  string `yaml:"serviceAccountID"`
	WorkspaceID       string `yaml:"workspaceID"`
}

// Enabled reports whether enough is configured to attempt federation. It
// deliberately mirrors claude.FederationConfig.Enabled so that validation and
// the client agree on what counts as configured -- otherwise a deployment can
// pass validation and then silently fall back to the API key path.
func (f *ClaudeFederationConfig) Enabled() bool {
	return f.IdentityTokenFile != "" && f.FederationRuleID != "" && f.OrganizationID != ""
}

// Load reads configuration from a file and environment variables
func Load(path string) (*Config, error) {
	config := &Config{}

	// Read config file
	data, err := os.ReadFile(path) //nolint:gosec // config path is supplied by the operator, not untrusted input
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Expand environment variables in the config file content
	// This allows using ${VAR_NAME} syntax in the YAML file
	expandedData := os.ExpandEnv(string(data))

	// Parse YAML
	if err := yaml.Unmarshal([]byte(expandedData), config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Override with environment variables if present
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config.Kubernetes.KubeConfig = kubeconfig
	}

	// API Key settings (for server authentication)
	if apiKey := os.Getenv("API_KEY"); apiKey != "" {
		config.Server.Auth.APIKey = apiKey
	}

	// Claude API settings
	if apiKey := os.Getenv("CLAUDE_API_KEY"); apiKey != "" {
		config.Claude.APIKey = apiKey
	}

	// Workload Identity Federation. These names match the ones the official
	// Anthropic SDKs read, so a workload configured for federation elsewhere
	// needs no separate wiring here.
	if v := os.Getenv("ANTHROPIC_IDENTITY_TOKEN_FILE"); v != "" {
		config.Claude.Federation.IdentityTokenFile = v
	}
	if v := os.Getenv("ANTHROPIC_FEDERATION_RULE_ID"); v != "" {
		config.Claude.Federation.FederationRuleID = v
	}
	if v := os.Getenv("ANTHROPIC_ORGANIZATION_ID"); v != "" {
		config.Claude.Federation.OrganizationID = v
	}
	if v := os.Getenv("ANTHROPIC_SERVICE_ACCOUNT_ID"); v != "" {
		config.Claude.Federation.ServiceAccountID = v
	}
	if v := os.Getenv("ANTHROPIC_WORKSPACE_ID"); v != "" {
		config.Claude.Federation.WorkspaceID = v
	}

	// ArgoCD settings
	if argoURL := os.Getenv("ARGOCD_SERVER"); argoURL != "" {
		config.ArgoCD.URL = argoURL
	}
	if argoToken := os.Getenv("ARGOCD_AUTH_TOKEN"); argoToken != "" {
		config.ArgoCD.AuthToken = argoToken
	}
	if argoUser := os.Getenv("ARGOCD_USERNAME"); argoUser != "" {
		config.ArgoCD.Username = argoUser
	}
	if argoPass := os.Getenv("ARGOCD_PASSWORD"); argoPass != "" {
		config.ArgoCD.Password = argoPass
	}

	// GitLab settings
	if gitlabURL := os.Getenv("GITLAB_URL"); gitlabURL != "" {
		config.GitLab.URL = gitlabURL
	}
	if gitlabToken := os.Getenv("GITLAB_AUTH_TOKEN"); gitlabToken != "" {
		config.GitLab.AuthToken = gitlabToken
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check server configuration
	if c.Server.Address == "" {
		return fmt.Errorf("server address is required")
	}

	if c.Server.ReadTimeout < 0 {
		return fmt.Errorf("server read timeout must be non-negative")
	}

	if c.Server.WriteTimeout < 0 {
		return fmt.Errorf("server write timeout must be non-negative")
	}

	// Check Claude configuration
	// Either a static key or a fully configured federation block is enough.
	// Requiring the key unconditionally would crash-loop a deployment that has
	// completed the move to Workload Identity Federation and removed it.
	if c.Claude.APIKey == "" && !c.Claude.Federation.Enabled() {
		return fmt.Errorf("claude authentication is required: set claude.apiKey, or configure claude.federation for Workload Identity Federation")
	}

	if c.Claude.ModelID == "" {
		return fmt.Errorf("claude model ID is required")
	}

	if c.Claude.BaseURL == "" {
		return fmt.Errorf("claude base URL is required")
	}

	if c.Claude.MaxTokens <= 0 {
		return fmt.Errorf("claude max tokens must be positive")
	}

	if c.Claude.MaxTokens > 8192 {
		return fmt.Errorf("claude max tokens cannot exceed 8192")
	}

	if c.Claude.Temperature < 0.0 || c.Claude.Temperature > 1.0 {
		return fmt.Errorf("claude temperature must be between 0.0 and 1.0")
	}

	// Check Kubernetes configuration
	if c.Kubernetes.InCluster && c.Kubernetes.KubeConfig != "" {
		return fmt.Errorf("cannot specify both inCluster=true and kubeconfig path")
	}

	// Validate ArgoCD configuration if URL is provided
	if c.ArgoCD.URL != "" {
		if c.ArgoCD.AuthToken == "" && (c.ArgoCD.Username == "" || c.ArgoCD.Password == "") {
			return fmt.Errorf("ArgoCD requires either authToken or username/password")
		}
	}

	// Validate GitLab configuration if URL is provided
	if c.GitLab.URL != "" && c.GitLab.AuthToken == "" {
		return fmt.Errorf("GitLab auth token is required when GitLab URL is provided")
	}

	return nil
}
