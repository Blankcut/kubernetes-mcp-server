# Kubernetes Claude MCP Helm Chart

This Helm chart deploys the Kubernetes Claude MCP server, which provides a Model Context Protocol (MCP) server that integrates Claude's AI capabilities with Kubernetes, ArgoCD, and GitLab.

## Prerequisites

- Kubernetes 1.16+
- Helm 3.0+
- Access to a Kubernetes cluster
- ArgoCD deployment (optional, but recommended for GitOps features)
- GitLab instance (optional, but recommended for GitOps features)
- An Anthropic API key for Claude

## Installing the Chart

Add the repository (if hosted):

```bash
helm repo add my-repo https://your-helm-repo-url
helm repo update
```

To install the chart with the release name `mcp-server`:

```bash
helm install mcp-server my-repo/kubernetes-claude-mcp \
  --set secrets.claude.apiKey=your-claude-api-key \
  --set secrets.argocd.authToken=your-argocd-token \
  --set secrets.gitlab.authToken=your-gitlab-token
```

Alternatively, if you're installing from a local directory:

```bash
helm install mcp-server ./kubernetes-claude-mcp \
  --set secrets.claude.apiKey=your-claude-api-key
```

## Using Existing Secrets

If you manage your secrets outside of Helm (recommended for production), you can reference them:

```bash
# Create secrets separately
kubectl create secret generic claude-secret --from-literal=api-key=your-claude-api-key
kubectl create secret generic argocd-secret --from-literal=auth-token=your-argocd-token
kubectl create secret generic gitlab-secret --from-literal=auth-token=your-gitlab-token

# Install chart referencing existing secrets
helm install mcp-server ./kubernetes-claude-mcp \
  --set existingSecrets.claude.name=claude-secret \
  --set existingSecrets.claude.key=api-key \
  --set existingSecrets.argocd.name=argocd-secret \
  --set existingSecrets.argocd.key=auth-token \
  --set existingSecrets.gitlab.name=gitlab-secret \
  --set existingSecrets.gitlab.key=auth-token
```

## Exposing the Service

By default, the service is of type `ClusterIP`. To expose it as a NodePort:

```bash
helm install mcp-server ./kubernetes-claude-mcp \
  --set service.type=NodePort \
  --set service.nodePort=30080
```

## Configuration

The following table lists the configurable parameters of the chart and their default values.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `your-registry/kubernetes-claude-mcp` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Image tag | `latest` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Kubernetes service port | `80` |
| `service.nodePort` | NodePort (if service.type is NodePort) | `nil` |
| `config.server.address` | Server address | `:8080` |
| `config.argocd.url` | ArgoCD server URL | `https://argocd.example.com` |
| `config.gitlab.url` | GitLab URL | `https://gitlab.com` |
| `config.claude.modelID` | Claude model ID | `claude-3-7-sonnet-20250219` |
| `secrets.claude.apiKey` | Claude API key | `""` |
| `secrets.argocd.authToken` | ArgoCD auth token | `""` |
| `secrets.gitlab.authToken` | GitLab auth token | `""` |
| `existingSecrets.claude.name` | Existing secret name for Claude | `""` |
| `existingSecrets.claude.key` | Key in existing secret for Claude | `""` |

You can specify each parameter using the `--set key=value[,key=value]` argument to `helm install`.

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart:

```bash
helm install mcp-server ./kubernetes-claude-mcp -f values.yaml
```

## Upgrading the Chart

To upgrade the deployment:

```bash
helm upgrade mcp-server ./kubernetes-claude-mcp
```

## Uninstalling the Chart

To uninstall/delete the deployment:

```bash
helm uninstall mcp-server
```