# Claude Kubernetes MCP Server

This repository contains the Claude Kubernetes MCP (Model Control Plane) server, built in Go. The server integrates with ArgoCD, GitLab, Claude AI, and Kubernetes to enable advanced control and automation of Kubernetes environments.

## Table of Contents
- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Setup Instructions](#setup-instructions)
- [Configuration](#configuration)
- [Running Locally](#running-locally)
- [Building and Running with Docker](#building-and-running-with-docker)
- [Production Deployment](#production-deployment)
- [API Documentation](#api-documentation)
- [Postman Collection](#postman-collection)
- [License](#license)

---

## Overview
This server is designed to orchestrate Kubernetes workloads using Claude AI, GitLab, ArgoCD, and Vault. It exposes a REST API that allows programmatic interaction with these systems, driven by a configured `config.yaml` and authenticated using an API key.

## Prerequisites
- Go 1.20+
- Docker
- Kubernetes cluster & valid `~/.kube/config`
- EKS cluster with AWS_PROFILE set locally
- ArgoCD credentials
- GitLab personal access token
- Claude API key (Anthropic)
- Vault credentials (optional, depending on use)

## Setup Instructions

### 1. Clone the Repository
```bash
git clone https://github.com/blankcut/kubernetes-mcp-server.git
cd kubernetes-mcp-server
```

### 2. Export Required Environment Variables
Export credentials for ArgoCD, GitLab, and Claude:
```bash
export ARGOCD_USERNAME="argocd-username"
export ARGOCD_PASSWORD="argocd-password"
export GITLAB_TOKEN="gitlab-token"
export CLAUDE_API_KEY="claude-api-key"
export VAULT_TOKEN="optional-if-using-vault"
```

Ensure a kubeconfig is available:
```bash
export KUBECONFIG=~/.kube/config
```

### 3. Configure `config.yaml`
Update `kubernetes-claude-mcp/config.yaml` with credentials and server preferences:
```yaml
server:
  address: ":8080"
  readTimeout: 30
  writeTimeout: 60
  auth:
    apiKey: ""${API_KEY}"" 

kubernetes:
  kubeconfig: ""
  inCluster: false
  defaultContext: ""
  defaultNamespace: "default"

argocd:
  url: "http://argocd.blankcut.com:30080"
  authToken: ""
  username: "${ARGOCD_USERNAME}"
  password: "${ARGOCD_PASSWORD}"
  insecure: true

gitlab:
  url: "https://gitlab.com"
  authToken: "${AUTH_TOKEN}"
  apiVersion: "v4"
  projectPath: ""${PROJECT_PATH}""

claude:
  apiKey: "${API_KEY}"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-3-haiku-20240307"
  maxTokens: 4096
  temperature: 0.7
```

You can use the provided Go templates or environment variable interpolation method.

### 4. Add API Key for Postman
Please ensure a `config.yaml` includes an `apiKey`. This will be used to authenticate requests in Postman or any external client.

---

## Running Locally
```bash
cd kubernetes-claude-mcp
go run ./cmd/server/main.go
```

### With Debug Logging:
```bash
LOG_LEVEL=debug go run ./cmd/server/main.go --config config.yaml
```

Server will start and bind to the configured port in `config.yaml` (default: 8080).

---

## Building and Running with Docker
### 1. Build the Image
```bash
cd kubernetes-claude-mcp
docker build -t claude-mcp-server -f Dockerfile .
```

### 2. Run the Container (second build option included)
```bash
cd kubernetes-claude-mcp
docker-compose build
docker-compose up -d
```

---

## Production Deployment

A Helm chart is included in the repository for Kubernetes deployment:

### 1. Navigate to the Helm Chart Directory
```bash
cd kubernetes-claude-mcp/deployments/helm
```

### 2. Deploy with Helm
Update `values.yaml` with appropriate values and run:
```bash
helm install claude-mcp .
```

To upgrade:
```bash
helm upgrade claude-mcp .
```

Please ensure secrets and config maps are properly mounted and secured in the cluster.

---

## API Documentation
Below are the primary endpoints exposed by the MCP server. All requests require the `X-API-Key` header:

### General
- **Health Check**
  - `GET /api/v1/health`

### Kubernetes
- **List Namespaces**
  - `GET /api/v1/namespaces`
- **List Resources**
  - `GET /api/v1/resources/{kind}?namespace={ns}`
- **Get Specific Resource**
  - `GET /api/v1/resources/{kind}/{name}?namespace={ns}`
- **Get Events for a Resource**
  - `GET /api/v1/events?namespace={ns}&resource={kind}&name={name}`

### ArgoCD
- **List Applications**
  - `GET /api/v1/argocd/applications`

### Claude MCP Endpoints
- **Analyze Resource**
  - `POST /api/v1/mcp/resource`
- **Troubleshoot Resource**
  - `POST /api/v1/mcp/troubleshoot`
- **Commit Analysis (GitLab)**
  - `POST /api/v1/mcp/commit`
- **Generic MCP Request**
  - `POST /api/v1/mcp`

All POST endpoints accept a JSON payload containing fields such as:
```json
{
  "resource": "pod",
  "name": "example-pod",
  "namespace": "default",
  "query": "What’s wrong with this pod?"
}
```

---

## Postman Collection
A ready-to-use Postman collection will be available soon.
---

## Donation
Please contribute to our coffee fund to help us continue to do great things [Buy Me Coffee](buymeacoffee.com/blankcut)

## License
This project is licensed under the [MIT License](./LICENSE).

---

## Contributing
Documentation will be expanded soon. If you’d like to contribute, feel free to open a pull request or file an issue!

