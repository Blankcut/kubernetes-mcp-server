---
title: Troubleshooting Resources
description: Learn how to use Kubernetes Claude MCP to diagnose and solve problems with your Kubernetes resources and applications.
date: 2025-03-01
order: 6
tags: ['troubleshooting', 'guides']
---

# Troubleshooting Resources

Kubernetes Claude MCP is a powerful tool for diagnosing and resolving issues in your Kubernetes environment. This guide will walk you through common troubleshooting scenarios and how to use the MCP server to address them.

## Getting Started with Troubleshooting

The `/api/v1/mcp/troubleshoot` endpoint is specifically designed for troubleshooting. It automatically:

1. Collects all relevant information about a resource
2. Detects common issues and their severity
3. Correlates information across systems (Kubernetes, ArgoCD, GitLab)
4. Generates recommendations for fixing the issues
5. Provides Claude AI-powered analysis of the problems

## Troubleshooting Common Resource Types

### Troubleshooting Pods

Pods are often the first place to look when troubleshooting application issues.

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "resource": "pod",
    "name": "my-app-pod",
    "namespace": "default",
    "query": "Why is this pod not starting?"
  }' \
  http://localhost:8080/api/v1/mcp/troubleshoot
```

**What MCP Detects:**

- Pod status issues (Pending, CrashLoopBackOff, ImagePullBackOff, etc.)
- Container status and restart counts
- Resource constraints (CPU/memory limits)
- Volume mounting issues
- Init container failures
- Image pull errors
- Scheduling problems
- Events related to the pod

**Example Troubleshooting Output:**

```json
{
  "success": true,
  "analysis": "The pod 'my-app-pod' is failing to start due to an ImagePullBackOff error. The container runtime is unable to pull the image 'myregistry.com/my-app:v1.2.3' because of authentication issues with the private registry. Looking at the events, there was an 'ErrImagePull' error with the message 'unauthorized: authentication required'...",
  "troubleshootResult": {
    "issues": [
      {
        "title": "Image Pull Error",
        "category": "ImagePullError",
        "severity": "Error",
        "source": "Kubernetes",
        "description": "Failed to pull image 'myregistry.com/my-app:v1.2.3': unauthorized: authentication required"
      }
    ],
    "recommendations": [
      "Create or update the ImagePullSecret for the private registry",
      "Verify the image name and tag are correct",
      "Check that the ServiceAccount has access to the ImagePullSecret"
    ]
  }
}
```

### Troubleshooting Deployments

Deployments manage replica sets and pods, so issues can occur at multiple levels.

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "resource": "deployment",
    "name": "my-app",
    "namespace": "default",
    "query": "Why are pods not scaling up?"
  }' \
  http://localhost:8080/api/v1/mcp/troubleshoot
```

**What MCP Detects:**

- ReplicaSet creation issues
- Pod scaling issues
- Resource quotas preventing scaling
- Node capacity issues
- Pod disruption budgets
- Deployment strategy issues
- Resource constraints on pods
- Health check configuration issues

**Example Troubleshooting Output:**

```json
{
  "success": true,
  "analysis": "The deployment 'my-app' is unable to scale up because the pods are requesting more CPU resources than are available in the cluster. The deployment is configured to request 2 CPU cores per pod, but the nodes in your cluster only have 1.8 cores available per node...",
  "troubleshootResult": {
    "issues": [
      {
        "title": "Insufficient CPU Resources",
        "category": "ResourceConstraint",
        "severity": "Warning",
        "source": "Kubernetes",
        "description": "Insufficient CPU resources available to schedule pods (requested: 2, available: 1.8)"
      }
    ],
    "recommendations": [
      "Reduce the CPU request in the deployment specification",
      "Add more nodes to the cluster or use nodes with more CPU capacity",
      "Check if there are any resource quotas preventing the scaling"
    ]
  }
}
```

### Troubleshooting Services

Services provide network connectivity between components, and issues often relate to selector mismatches or port configurations.

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "resource": "service",
    "name": "my-app-service",
    "namespace": "default",
    "query": "Why can't I connect to this service?"
  }' \
  http://localhost:8080/api/v1/mcp/troubleshoot
```

**What MCP Detects:**

- Selector mismatches between service and pods
- Port configuration issues
- Endpoint availability
- Pod readiness issues
- Network policy restrictions
- Service type misconfigurations
- External name resolution issues (for ExternalName services)

**Example Troubleshooting Output:**

```json
{
  "success": true,
  "analysis": "The service 'my-app-service' is not working correctly because there are no endpoints being selected. The service uses the selector 'app=my-app,tier=frontend', but examining the pods in the namespace, I can see that the pods have the labels 'app=my-app,tier=web'. The mismatch in the 'tier' label (frontend vs web) is preventing the service from selecting any pods...",
  "troubleshootResult": {
    "issues": [
      {
        "title": "Selector Mismatch",
        "category": "ServiceSelectorIssue",
        "severity": "Error",
        "source": "Kubernetes",
        "description": "Service selector 'app=my-app,tier=frontend' doesn't match any pods (pods have 'app=my-app,tier=web')"
      }
    ],
    "recommendations": [
      "Update the service selector to match the actual pod labels: 'app=my-app,tier=web'",
      "Alternatively, update the pod labels to match the service selector",
      "Verify that pods are in the 'Running' state and passing readiness probes"
    ]
  }
}
```

### Troubleshooting Ingresses

Ingress resources configure external access to services, and issues often relate to hostname mismatches or TLS configuration.

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "resource": "ingress",
    "name": "my-app-ingress",
    "namespace": "default",
    "query": "Why is this ingress returning 404 errors?"
  }' \
  http://localhost:8080/api/v1/mcp/troubleshoot
```

**What MCP Detects:**

- Backend service existence and configuration
- Path routing rules
- TLS certificate issues
- Ingress controller availability
- Host name configurations
- Annotation misconfigurations
- Service port mappings

## Troubleshooting GitOps Resources

Kubernetes Claude MCP excels at diagnosing issues in GitOps workflows by correlating information between Kubernetes, ArgoCD, and GitLab.

### Troubleshooting ArgoCD Applications

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "resource": "application",
    "name": "my-argocd-app",
    "namespace": "argocd",
    "query": "Why is this application out of sync?"
  }' \
  http://localhost:8080/api/v1/mcp/troubleshoot
```

**What MCP Detects:**

- Sync status issues
- Sync history and recent failures
- Git repository connectivity issues
- Manifest validation errors
- Resource differences between desired and actual state
- Health status issues
- Related Kubernetes resources

**Example Troubleshooting Output:**

```json
{
  "success": true,
  "analysis": "The ArgoCD application 'my-argocd-app' is out of sync because there are local changes to the Deployment resource that differ from the version in Git. Specifically, someone has manually scaled the deployment from 3 replicas (as defined in Git) to 5 replicas using kubectl...",
  "troubleshootResult": {
    "issues": [
      {
        "title": "Manual Modification",
        "category": "SyncIssue",
        "severity": "Warning",
        "source": "ArgoCD",
        "description": "Deployment 'my-app' was manually modified: replicas changed from 3 to 5"
      }
    ],
    "recommendations": [
      "Use 'argocd app sync my-argocd-app' to revert to the state defined in Git",
      "Update the Git repository to reflect the desired replica count",
      "Enable self-healing in the ArgoCD application to prevent manual modifications"
    ]
  }
}
```

### Investigating Commit Impact

When a deployment fails after a GitLab commit, you can analyze the commit's impact:

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "projectId": "mygroup/myproject",
    "commitSha": "abcdef1234567890",
    "query": "How has this commit affected Kubernetes resources and what issues has it caused?"
  }' \
  http://localhost:8080/api/v1/mcp/commit
```

**What MCP Analyzes:**

- Files changed in the commit
- Connected ArgoCD applications
- Affected Kubernetes resources
- Subsequent pipeline results
- Changes in resource configurations
- Introduction of new errors or warnings

## Advanced Troubleshooting Scenarios

### Multi-Resource Analysis

You can troubleshoot complex issues by instructing Claude to correlate multiple resources:

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "query": "Analyze the connectivity issue between the frontend deployment and the backend service in the 'myapp' namespace. Check both the deployment and the service configurations."
  }' \
  http://localhost:8080/api/v1/mcp
```

### Diagram Generation

For complex troubleshooting scenarios, you can request diagram generation to visualize relationships:

**Example Request:**

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "resource": "deployment",
    "name": "my-app",
    "namespace": "default",
    "query": "Create a diagram showing this deployment's relationship to all associated resources, including services, ingresses, configmaps, and secrets."
  }' \
  http://localhost:8080/api/v1/mcp/resource
```

Claude can generate Mermaid diagrams within its response to visualize the relationships.

## Troubleshooting Best Practices

When using Kubernetes Claude MCP for troubleshooting:

1. **Start specific**: Begin with the resource that's showing symptoms
2. **Go broad**: If needed, expand to related resources
3. **Use specific queries**: The more specific your query, the better Claude can help
4. **Include context**: Mention what you've already tried or specific symptoms
5. **Follow recommendations**: Try the recommended fixes one at a time
6. **Iterate**: Use follow-up queries to dive deeper

## Real-Time Troubleshooting

For ongoing issues, you can set up continuous monitoring:

```bash
# Watch a resource and get alerts when issues are detected
watch -n 30 'curl -s -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d "{\"resource\":\"deployment\",\"name\":\"my-app\",\"namespace\":\"default\",\"query\":\"Report any new issues\"}" \
  http://localhost:8080/api/v1/mcp/troubleshoot | jq .troubleshootResult.issues'
```

## Troubleshooting Reference

Here's a quick reference of what to check for common Kubernetes issues:

| Symptom | Resource to Check | Common Issues |
|---------|-------------------|---------------|
| Application not starting | Pod | Image pull errors, resource constraints, configuration issues |
| Cannot connect to app | Service | Selector mismatch, port configuration, pod health |
| External access failing | Ingress | Path configuration, backend service, TLS issues |
| Scaling issues | Deployment | Resource constraints, pod disruption budgets, affinity rules |
| Configuration issues | ConfigMap/Secret | Missing keys, invalid format, mounting issues |
| Persistent storage issues | PVC | Storage class, capacity issues, access modes |
| GitOps sync failures | ArgoCD Application | Git repo issues, manifest errors, resource conflicts |