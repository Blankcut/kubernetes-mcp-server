---
title: Configuration Guide
description: Learn how to configure and customize the Kubernetes Claude MCP server to suit your needs and environment.
date: 2025-03-01
order: 4
tags: ['configuration', 'setup']
---

# Configuration Guide

This guide explains how to configure Kubernetes Claude MCP to work optimally in your environment. The server is highly configurable, allowing you to customize its behavior and integrations.

## Configuration File

Kubernetes Claude MCP is primarily configured using a YAML file (`config.yaml`). This file contains settings for the server, Kubernetes connection, ArgoCD integration, GitLab integration, and Claude AI.

Here's a complete example of the configuration file with explanations:

```yaml
# Server configuration
server:
  # Address to bind the server on (host:port)
  address: ":8080"
  # Read timeout in seconds
  readTimeout: 30
  # Write timeout in seconds
  writeTimeout: 60
  # Authentication settings
  auth:
    # API key for authenticating requests
    apiKey: "your_api_key_here"

# Kubernetes connection settings
kubernetes:
  # Path to kubeconfig file (leave empty for in-cluster)
  kubeconfig: ""
  # Whether to use in-cluster config
  inCluster: false
  # Default Kubernetes context (leave empty for current)
  defaultContext: ""
  # Default namespace
  defaultNamespace: "default"

# ArgoCD integration settings
argocd:
  # ArgoCD server URL
  url: "https://argocd.example.com"
  # ArgoCD auth token (optional if using username/password)
  authToken: ""
  # ArgoCD username (optional if using token)
  username: "admin"
  # ArgoCD password (optional if using token)
  password: "password"
  # Whether to allow insecure connections
  insecure: false

# GitLab integration settings
gitlab:
  # GitLab server URL
  url: "https://gitlab.com"
  # GitLab personal access token
  authToken: "your_gitlab_token"
  # GitLab API version
  apiVersion: "v4"
  # Default project path
  projectPath: "namespace/project"

# Claude AI settings
claude:
  # Claude API key
  apiKey: "your_claude_api_key"
  # Claude API base URL
  baseURL: "https://api.anthropic.com"
  # Claude model ID
  modelID: "claude-sonnet-4.5-20250514"
  # Maximum tokens for Claude responses
  maxTokens: 8192
  # Temperature for Claude responses (0.0-1.0)
  temperature: 0.3
```

## Configuration Options

### Server Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `address` | Host and port to bind the server (":8080" means all interfaces, port 8080) | ":8080" |
| `readTimeout` | HTTP read timeout in seconds | 30 |
| `writeTimeout` | HTTP write timeout in seconds | 60 |
| `auth.apiKey` | API key for authenticating requests | - |

### Kubernetes Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `kubeconfig` | Path to kubeconfig file | "" (auto-detect) |
| `inCluster` | Whether to use in-cluster configuration | false |
| `defaultContext` | Default Kubernetes context | "" (current context) |
| `defaultNamespace` | Default namespace for operations | "default" |

### ArgoCD Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `url` | ArgoCD server URL | - |
| `authToken` | ArgoCD auth token | "" |
| `username` | ArgoCD username | "" |
| `password` | ArgoCD password | "" |
| `insecure` | Allow insecure connections to ArgoCD | false |

### GitLab Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `url` | GitLab server URL | "https://gitlab.com" |
| `authToken` | GitLab personal access token | - |
| `apiVersion` | GitLab API version | "v4" |
| `projectPath` | Default project path | "" |

### Claude Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `apiKey` | Claude API key | - |
| `baseURL` | Claude API base URL | "https://api.anthropic.com" |
| `modelID` | Claude model ID | "claude-sonnet-4.5-20250514" |
| `maxTokens` | Maximum tokens for response | 8192 |
| `temperature` | Temperature for responses (0.0-1.0) | 0.3 |

## Environment Variables

In addition to the configuration file, you can use environment variables to override any configuration option. This is especially useful for secrets and credentials.

Environment variables follow this pattern:

- For server options: `SERVER_OPTION_NAME`
- For Kubernetes options: `KUBERNETES_OPTION_NAME`
- For ArgoCD options: `ARGOCD_OPTION_NAME`
- For GitLab options: `GITLAB_OPTION_NAME`
- For Claude options: `CLAUDE_OPTION_NAME`

Common examples:

```bash
# API keys
export CLAUDE_API_KEY=your_claude_api_key
export API_KEY=your_api_key_for_server

# ArgoCD credentials
export ARGOCD_USERNAME=your_argocd_username
export ARGOCD_PASSWORD=your_argocd_password

# GitLab credentials
export GITLAB_AUTH_TOKEN=your_gitlab_token
```

## Variable Interpolation

The configuration file supports variable interpolation, allowing you to reference environment variables in your config. This is useful for injecting secrets:

```yaml
server:
  auth:
    apiKey: "${API_KEY}"

claude:
  apiKey: "${CLAUDE_API_KEY}"
```

## Configuration Hierarchy

The server reads configuration in the following order (later overrides earlier):

1. Default values
2. Configuration file
3. Environment variables

This allows you to have a base configuration file and override specific settings with environment variables.

## ArgoCD Integration

### Authentication Methods

There are two ways to authenticate with ArgoCD:

1. **Token-based authentication**: Provide an auth token in `argocd.authToken`.
2. **Username/password authentication**: Provide username and password in `argocd.username` and `argocd.password`.

For production environments, token-based authentication is recommended for security.

### Insecure Mode

If you're using a self-signed certificate for ArgoCD, you can set `argocd.insecure` to `true` to skip certificate validation. However, this is not recommended for production environments.

## GitLab Integration

### Personal Access Token

To integrate with GitLab, you need a personal access token with the following scopes:

- `read_api` - For accessing repository information
- `read_repository` - For accessing repository content
- `read_registry` - For accessing container registry (if needed)

### Self-hosted GitLab

If you're using a self-hosted GitLab instance, set the `gitlab.url` to your GitLab URL:

```yaml
gitlab:
  url: "https://gitlab.your-company.com"
  # Other GitLab settings...
```

## Claude AI Configuration

### Model Selection

Kubernetes Claude MCP supports different Claude model variants. The default is `claude-sonnet-4.5-20250514` (Claude Sonnet 4.5), but you can choose others based on your needs:

- `claude-sonnet-4.5-20250514` - Latest and most capable model (recommended)
- `claude-sonnet-4-20250514` - Claude Sonnet 4, excellent performance
- `claude-3-5-sonnet-20241022` - Claude 3.5 Sonnet, balanced performance
- `claude-3-opus-20240229` - Claude 3 Opus, good for complex analysis
- `claude-3-haiku-20240307` - Claude 3 Haiku, fastest model

### Response Parameters

You can adjust two parameters that affect Claude's responses:

1. `maxTokens` - Maximum number of tokens in the response (1-8192)
2. `temperature` - Controls randomness in responses (0.0-1.0)
   - Lower values (e.g., 0.3) make responses more deterministic and focused
   - Higher values (e.g., 0.7) make responses more creative

For troubleshooting and analysis, a temperature of 0.3-0.5 is recommended.

## Advanced Configuration

### Running Behind a Proxy

If the server needs to connect to external services through a proxy, set the standard HTTP proxy environment variables:

```bash
export HTTP_PROXY=http://proxy.example.com:8080
export HTTPS_PROXY=http://proxy.example.com:8080
export NO_PROXY=localhost,127.0.0.1,.cluster.local
```

### TLS Configuration

For production deployments, it's recommended to use TLS. This is typically handled by your ingress controller, load balancer, or API gateway.

If you need to terminate TLS at the server (not recommended for production), you can use a reverse proxy like Nginx or Traefik.

### Logging Configuration

The logging level can be controlled with the `LOG_LEVEL` environment variable:

```bash
export LOG_LEVEL=debug  # debug, info, warn, error
```

For production, `info` is recommended. Use `debug` only for troubleshooting.

## Configuration Examples

### Minimal Configuration

```yaml
server:
  address: ":8080"
  auth:
    apiKey: "your_api_key_here"

kubernetes:
  inCluster: false

claude:
  apiKey: "your_claude_api_key"
  modelID: "claude-sonnet-4.5-20250514"
```

### Production Kubernetes Configuration

```yaml
server:
  address: ":8080"
  readTimeout: 60
  writeTimeout: 120
  auth:
    apiKey: "${API_KEY}"

kubernetes:
  inCluster: true
  defaultNamespace: "default"

argocd:
  url: "https://argocd.example.com"
  authToken: "${ARGOCD_AUTH_TOKEN}"
  insecure: false

gitlab:
  url: "https://gitlab.example.com"
  authToken: "${GITLAB_AUTH_TOKEN}"
  apiVersion: "v4"

claude:
  apiKey: "${CLAUDE_API_KEY}"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-sonnet-4.5-20250514"
  maxTokens: 8192
  temperature: 0.3
```

## Troubleshooting Configuration

If you encounter issues with your configuration:

1. Check that all required fields are set correctly
2. Verify that environment variables are correctly set and accessible to the server
3. Test connectivity to external services (Kubernetes, ArgoCD, GitLab)
4. Check the server logs for error messages
5. Ensure your Claude API key is valid and has sufficient quota

### Common Issues

#### "Failed to create Kubernetes client"

This usually indicates an issue with the Kubernetes configuration:

- Check if the kubeconfig file exists and is accessible
- Verify the permissions of the kubeconfig file
- For in-cluster config, ensure the pod has the proper service account

#### "Failed to connect to ArgoCD"

ArgoCD connectivity issues are typically related to:

- Incorrect URL or credentials
- Network connectivity issues
- Certificate validation (if `insecure: false`)

Try using the `--log-level=debug` flag to get more details:

```bash
LOG_LEVEL=debug ./mcp-server --config config.yaml
```

#### "Failed to connect to GitLab"

GitLab connectivity issues may be due to:

- Invalid personal access token
- Insufficient permissions for the token
- Network connectivity issues

#### "Claude API error"

Claude API errors usually indicate:

- Invalid API key
- Rate limiting or quota issues
- Incorrect model ID

## Updating Configuration

You can update the configuration without restarting the server by sending a SIGHUP signal:

```bash
# Find the process ID
ps aux | grep mcp-server

# Send SIGHUP signal
kill -HUP <process_id>
```

For containerized deployments, you'll need to restart the container to apply configuration changes.

## Next Steps

Now that you've configured Kubernetes Claude MCP, you can:

- [Explore the API](/docs/api-overview) to learn how to interact with the server
- [Try some examples](/docs/examples/basic-usage) to see common use cases
- [Learn about troubleshooting](/docs/troubleshooting-resources) to diagnose issues in your cluster