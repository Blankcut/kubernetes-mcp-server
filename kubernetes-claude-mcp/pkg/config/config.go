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
}

// Load reads configuration from a file and environment variables
func Load(path string) (*Config, error) {
	config := &Config{}

	// Read config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Override with environment variables if present
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config.Kubernetes.KubeConfig = kubeconfig
	}

	// Claude API settings
	if apiKey := os.Getenv("CLAUDE_API_KEY"); apiKey != "" {
		config.Claude.APIKey = apiKey
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
	if c.Claude.APIKey == "" {
		return fmt.Errorf("claude API key is required")
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
