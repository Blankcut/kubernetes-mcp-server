# README Badges

Add these badges to the top of your README.md file (after the logo):

```markdown
# Kubernetes MCP Server

[![CI](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/ci.yml)
[![Release](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/release.yml/badge.svg)](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/release.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/blankcut/kubernetes-mcp-server)](https://hub.docker.com/r/blankcut/kubernetes-mcp-server)
[![Docker Image Size](https://img.shields.io/docker/image-size/blankcut/kubernetes-mcp-server/latest)](https://hub.docker.com/r/blankcut/kubernetes-mcp-server)
[![Go Report Card](https://goreportcard.com/badge/github.com/Blankcut/kubernetes-mcp-server)](https://goreportcard.com/report/github.com/Blankcut/kubernetes-mcp-server)
[![License](https://img.shields.io/github/license/Blankcut/kubernetes-mcp-server)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/Blankcut/kubernetes-mcp-server)](https://github.com/Blankcut/kubernetes-mcp-server/releases)
[![codecov](https://codecov.io/gh/Blankcut/kubernetes-mcp-server/branch/main/graph/badge.svg)](https://codecov.io/gh/Blankcut/kubernetes-mcp-server)

> A Model Context Protocol (MCP) server for Kubernetes cluster management with ArgoCD and GitLab integration.

## Quick Start

### Using Docker (Recommended)

```bash
docker pull blankcut/kubernetes-mcp-server:latest

docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v ~/.kube/config:/root/.kube/config:ro \
  blankcut/kubernetes-mcp-server:latest
```

### Using Docker Compose

```bash
docker-compose up -d
```

### Using Helm (Kubernetes)

```bash
helm repo add blankcut https://blankcut.github.io/kubernetes-mcp-server
helm install kubernetes-mcp-server blankcut/kubernetes-mcp-server
```

## Features

- Model Context Protocol (MCP) integration with Claude AI
- Kubernetes cluster management and monitoring
- ArgoCD GitOps integration
- GitLab CI/CD integration
- Resource correlation and troubleshooting
- Multi-architecture support (AMD64, ARM64)
```

## Installation

These badges will automatically update based on your repository status:
- **CI Badge**: Shows build status
- **Release Badge**: Shows release workflow status
- **Docker Pulls**: Shows how many times your image has been pulled
- **Docker Image Size**: Shows the size of your Docker image
- **Go Report Card**: Shows code quality score
- **License**: Shows your project license
- **GitHub Release**: Shows latest release version
- **Codecov**: Shows test coverage percentage

You can copy and paste this section into your README.md file.

