---
title: Quick Start Guide
description: Get started with Kubernetes Claude MCP in minutes with this quick start guide.
date: 2025-03-01
order: 2
tags: ['quickstart', 'installation']
---

# Quick Start Guide

This guide will help you get Kubernetes Claude MCP up and running quickly. We'll use Docker Compose for a simple local deployment.

## Prerequisites

- Docker and Docker Compose
- A Kubernetes cluster with kubectl configured
- ArgoCD instance (optional)
- GitLab access (optional)
- Claude API key from Anthropic

## Step 1: Clone the Repository

```bash
git clone https://github.com/blankcut/kubernetes-mcp-server.git
cd kubernetes-mcp-server
```

## Step 2: Configure Environment Variables

Create a `.env` file in the project directory:

```bash
# Required
CLAUDE_API_KEY=your_claude_api_key

# Optional ArgoCD credentials
ARGOCD_URL=http://argocd.your-domain.com
ARGOCD_USERNAME=admin
ARGOCD_PASSWORD=your_password
# Or use a token instead
# ARGOCD_AUTH_TOKEN=your_argocd_token

# Optional GitLab credentials
GITLAB_URL=https://gitlab.com
GITLAB_AUTH_TOKEN=your_gitlab_token

# Security
API_KEY=your_chosen_api_key_for_server_access
```

## Step 3: Create Configuration File

Create a `config.yaml` file:

```yaml
server:
  address: ":8080"
  readTimeout: 30
  writeTimeout: 60
  auth:
    apiKey: "${API_KEY}"

kubernetes:
  kubeconfig: ""
  inCluster: false
  defaultContext: ""
  defaultNamespace: "default"

argocd:
  url: "${ARGOCD_URL}"
  authToken: "${ARGOCD_AUTH_TOKEN}"
  username: "${ARGOCD_USERNAME}"
  password: "${ARGOCD_PASSWORD}"
  insecure: true

gitlab:
  url: "${GITLAB_URL}"
  authToken: "${GITLAB_AUTH_TOKEN}"
  apiVersion: "v4"
  projectPath: ""

claude:
  apiKey: "${CLAUDE_API_KEY}"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-sonnet-4.5-20250514"
  maxTokens: 8192
  temperature: 0.3
```

## Step 4: Start the Server

Run the server using Docker Compose:

```bash
docker-compose up -d
```

## Step 5: Verify the Installation

Check if the server is running:

```bash
curl http://localhost:8080/api/v1/health
```

You should see a JSON response with the server status and service availability.

## Step 6: Make Your First API Call

Let's query a pod in your Kubernetes cluster:

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_chosen_api_key" \
  -d '{
    "action": "queryResource",
    "resource": "pod",
    "name": "your-pod-name",
    "namespace": "your-namespace",
    "query": "What is the status of this pod and are there any issues?"
  }' \
  http://localhost:8080/api/v1/mcp/resource
```

The server will analyze the pod and return Claude's analysis, highlighting any issues and providing recommendations.

## Next Steps

Now that you have Kubernetes Claude MCP running, you can:

1. [Configure the server](/docs/configuration) for your specific environment
2. [Explore the API](/docs/api-overview) to learn about all available endpoints
3. Check out the [Troubleshooting Resources](/docs/troubleshooting-resources) guide for common use cases
4. Learn about [GitOps Integration](/docs/gitops-integration) with ArgoCD and GitLab

For a more detailed setup and configuration, see the [Installation Guide](/docs/installation).