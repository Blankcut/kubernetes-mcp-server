---
title: API Overview
description: Comprehensive documentation of the Kubernetes Claude MCP REST API endpoints, parameters, and response formats.
date: 2025-03-01
order: 5
tags: ['api', 'reference']
---

# API Overview

Kubernetes Claude MCP provides a comprehensive REST API for interacting with Kubernetes resources, ArgoCD applications, GitLab repositories, and Claude's AI capabilities. This document provides an overview of all available endpoints, their parameters, and response formats.

## API Structure

The API is organized into the following sections:

- **General**: Health check and general information
- **Kubernetes API**: Direct access to Kubernetes resources
- **ArgoCD API**: Access to ArgoCD applications and sync status
- **MCP API**: AI-powered analysis and troubleshooting

All API calls require authentication using an API key, which is passed in the `X-API-Key` header or as a bearer token in the `Authorization` header.

```bash
# Using X-API-Key header
curl -H "X-API-Key: your_api_key" https://mcp.example.com/api/v1/health

# Using Authorization header
curl -H "Authorization: Bearer your_api_key" https://mcp.example.com/api/v1/health
```

## Health Check

### GET /api/v1/health

Check the health status of the server and its connected services.

**Response:**

```json
{
  "status": "ok",
  "services": {
    "kubernetes": "available",
    "argocd": "available",
    "gitlab": "available",
    "claude": "assumed available"
  }
}
```

The `status` field will be `ok` if all required services are available, or `degraded` if some services are unavailable.

## Kubernetes API

### GET /api/v1/namespaces

List all namespaces in the Kubernetes cluster.

**Response:**

```json
{
  "namespaces": [
    "default",
    "kube-system",
    "monitoring",
    "argocd"
  ]
}
```

### GET /api/v1/resources/{kind}?namespace={ns}

List all resources of a specific kind, optionally filtered by namespace.

**Parameters:**
- `kind`: The Kubernetes resource kind (e.g., `pod`, `deployment`, `service`)
- `namespace`: (Optional) The namespace to filter by

**Response:**

```json
{
  "resources": [
    {
      "apiVersion": "v1",
      "kind": "Pod",
      "metadata": {
        "name": "example-pod",
        "namespace": "default",
        "...": "..."
      },
      "spec": { "...": "..." },
      "status": { "...": "..." }
    },
    // More resources...
  ]
}
```

### GET /api/v1/resources/{kind}/{name}?namespace={ns}

Get a specific resource by kind, name, and namespace.

**Parameters:**
- `kind`: The Kubernetes resource kind
- `name`: The resource name
- `namespace`: (Optional) The namespace of the resource

**Response:**

```json
{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "name": "example-pod",
    "namespace": "default",
    "...": "..."
  },
  "spec": { "...": "..." },
  "status": { "...": "..." }
}
```

### GET /api/v1/events?namespace={ns}&resource={kind}&name={name}

Get events related to a specific resource.

**Parameters:**
- `namespace`: The namespace of the resource
- `resource`: The resource kind
- `name`: The resource name

**Response:**

```json
{
  "events": [
    {
      "reason": "Created",
      "message": "Created container nginx",
      "type": "Normal",
      "count": 1,
      "firstTime": "2025-03-01T12:00:00Z",
      "lastTime": "2025-03-01T12:00:00Z",
      "object": {
        "kind": "Pod",
        "name": "example-pod",
        "namespace": "default"
      }
    },
    // More events...
  ]
}
```

## ArgoCD API

### GET /api/v1/argocd/applications

List all ArgoCD applications.

**Response:**

```json
{
  "applications": [
    {
      "metadata": {
        "name": "example-app",
        "namespace": "argocd"
      },
      "spec": {
        "source": {
          "repoURL": "https://github.com/example/repo.git",
          "path": "manifests",
          "targetRevision": "HEAD"
        },
        "destination": {
          "server": "https://kubernetes.default.svc",
          "namespace": "default"
        }
      },
      "status": {
        "sync": {
          "status": "Synced"
        },
        "health": {
          "status": "Healthy"
        }
      }
    },
    // More applications...
  ]
}
```

### GET /api/v1/argocd/applications/{name}

Get a specific ArgoCD application by name.

**Parameters:**
- `name`: The ArgoCD application name

**Response:**

```json
{
  "metadata": {
    "name": "example-app",
    "namespace": "argocd"
  },
  "spec": {
    "source": {
      "repoURL": "https://github.com/example/repo.git",
      "path": "manifests",
      "targetRevision": "HEAD"
    },
    "destination": {
      "server": "https://kubernetes.default.svc",
      "namespace": "default"
    }
  },
  "status": {
    "sync": {
      "status": "Synced"
    },
    "health": {
      "status": "Healthy"
    }
  }
}
```

## MCP API

The MCP API provides access to Claude's AI capabilities for analyzing Kubernetes resources and GitOps workflows.

### POST /api/v1/mcp

Generic MCP request for Claude analysis.

**Request:**

```json
{
  "action": "string",
  "resource": "string",
  "name": "string",
  "namespace": "string",
  "query": "string",
  "commitSha": "string",
  "projectId": "string",
  "resourceSpecs": {},
  "context": "string"
}
```

**Response:**

```json
{
  "success": true,
  "message": "Successfully processed request",
  "analysis": "Detailed analysis from Claude...",
  "actions": ["suggested actions..."],
  "context": {}
}
```

### POST /api/v1/mcp/resource

Analyze a specific Kubernetes resource.

**Request:**

```json
{
  "resource": "pod",
  "name": "example-pod",
  "namespace": "default",
  "query": "Is this pod healthy? If not, what are the issues?"
}
```

**Response:**

```json
{
  "success": true,
  "message": "Successfully processed queryResource request",
  "analysis": "Detailed analysis of the pod's health status...",
  "context": {
    "kind": "Pod",
    "name": "example-pod",
    "namespace": "default",
    "argoApplication": {},
    "gitlabProject": {},
    "events": []
  }
}
```

### POST /api/v1/mcp/commit

Analyze the impact of a specific GitLab commit.

**Request:**

```json
{
  "projectId": "group/project",
  "commitSha": "abcdef123456",
  "query": "What changes were made in this commit and how do they affect the deployed resources?"
}
```

**Response:**

```json
{
  "success": true,
  "message": "Successfully processed queryCommit request",
  "analysis": "Detailed analysis of the commit and its impact...",
  "context": {
    "commit": {},
    "affectedResources": []
  }
}
```

### POST /api/v1/mcp/troubleshoot

Troubleshoot a specific Kubernetes resource.

**Request:**

```json
{
  "resource": "deployment",
  "name": "example-deployment",
  "namespace": "default",
  "query": "Why is this deployment not scaling properly?"
}
```

**Response:**

```json
{
  "success": true,
  "message": "Successfully processed troubleshoot request",
  "analysis": "Detailed troubleshooting analysis...",
  "troubleshootResult": {
    "issues": [
      {
        "title": "Resource Constraint",
        "category": "ResourceIssue",
        "severity": "Warning",
        "source": "Kubernetes",
        "description": "Deployment cannot scale due to insufficient CPU resources"
      }
    ],
    "recommendations": [
      "Increase CPU request to allow for additional replicas",
      "Check node resources to ensure sufficient capacity"
    ]
  }
}
```

## Error Handling

All API endpoints return standard HTTP status codes:

- `200 OK`: Request was successful
- `400 Bad Request`: Invalid request format or parameters
- `401 Unauthorized`: Missing or invalid API key
- `404 Not Found`: Resource not found
- `500 Internal Server Error`: Server error

Error responses include a JSON body with details:

```json
{
  "error": "Failed to get resource",
  "details": "pod 'example-pod' not found in namespace 'default'"
}
```

## Pagination

For endpoints that return collections, pagination is supported using the following query parameters:

- `limit`: Maximum number of items to return (default: 100)
- `page`: Page number to return (default: 1)

Example:

```
GET /api/v1/resources/pods?namespace=default&limit=10&page=2
```

Response includes pagination metadata:

```json
{
  "resources": [...],
  "pagination": {
    "total": 25,
    "pages": 3,
    "currentPage": 2,
    "limit": 10
  }
}
```

## API Versioning

The API version is included in the URL path (`/api/v1/`). Future API versions will be made available at different paths (e.g., `/api/v2/`) to ensure backward compatibility.

## Rate Limiting

The API implements rate limiting to prevent abuse. Rate limits vary by endpoint:

- General endpoints: 100 requests per minute
- Kubernetes endpoints: 60 requests per minute
- ArgoCD endpoints: 60 requests per minute
- MCP endpoints: 20 requests per minute

Rate limit information is included in response headers:

- `X-RateLimit-Limit`: Total requests allowed per minute
- `X-RateLimit-Remaining`: Remaining requests in the current window
- `X-RateLimit-Reset`: Seconds until the rate limit resets