---
title: Installation Guide
description: Comprehensive guide for installing and configuring the Kubernetes Claude MCP server in various environments.
date: 2025-03-01
order: 3
tags: ['installation', 'deployment']
---

# Installation Guide

This guide provides detailed instructions for installing Kubernetes Claude MCP in different environments. Choose the method that best suits your needs.

## Prerequisites

Before installing Kubernetes Claude MCP, ensure you have:

- Access to a Kubernetes cluster (v1.19+)
- kubectl configured to access your cluster
- Claude API key from Anthropic
- Optional: ArgoCD instance (for GitOps integration)
- Optional: GitLab access (for commit analysis)

## Installation Methods

There are several ways to install Kubernetes Claude MCP:

1. [Docker Compose](#docker-compose) (for development/testing)
2. [Kubernetes Deployment](#kubernetes-deployment) (recommended for production)
3. [Helm Chart](#helm-chart) (easiest for Kubernetes)
4. [Manual Binary](#manual-binary) (for custom environments)

## Docker Compose

Docker Compose is ideal for local development and testing.

### Step 1: Clone the Repository

```bash
git clone https://github.com/blankcut/kubernetes-mcp-server.git
cd kubernetes-mcp-server
```

### Step 2: Configure Environment Variables

Create a `.env` file with your credentials:

```bash
CLAUDE_API_KEY=your_claude_api_key
ARGOCD_USERNAME=your_argocd_username
ARGOCD_PASSWORD=your_argocd_password
GITLAB_AUTH_TOKEN=your_gitlab_token
API_KEY=your_api_key_for_server_access
```

### Step 3: Configure the Server

Create or modify `config.yaml`:

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
  projectPath: "${PROJECT_PATH}"

claude:
  apiKey: "${CLAUDE_API_KEY}"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-sonnet-4.5-20250514"
  maxTokens: 8192
  temperature: 0.3
```

### Step 4: Start the Service

```bash
docker-compose up -d
```

The server will be available at http://localhost:8080.

## Kubernetes Deployment

For production environments, deploying to Kubernetes is recommended.

### Step 1: Create a Namespace

```bash
kubectl create namespace mcp-system
```

### Step 2: Create Secrets

```bash
kubectl create secret generic mcp-secrets \
  --namespace mcp-system \
  --from-literal=claude-api-key=your_claude_api_key \
  --from-literal=argocd-username=your_argocd_username \
  --from-literal=argocd-password=your_argocd_password \
  --from-literal=gitlab-token=your_gitlab_token \
  --from-literal=api-key=your_api_key_for_server_access
```

### Step 3: Create ConfigMap

```bash
kubectl create configmap mcp-config \
  --namespace mcp-system \
  --from-file=config.yaml
```

### Step 4: Apply Deployment Manifest

Create a file named `deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-mcp-server
  namespace: mcp-system
  labels:
    app: kubernetes-mcp-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubernetes-mcp-server
  template:
    metadata:
      labels:
        app: kubernetes-mcp-server
    spec:
      serviceAccountName: mcp-service-account
      containers:
      - name: server
        image: blankcut/kubernetes-mcp-server:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
        env:
        - name: CLAUDE_API_KEY
          valueFrom:
            secretKeyRef:
              name: mcp-secrets
              key: claude-api-key
        - name: ARGOCD_USERNAME
          valueFrom:
            secretKeyRef:
              name: mcp-secrets
              key: argocd-username
              optional: true
        - name: ARGOCD_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mcp-secrets
              key: argocd-password
              optional: true
        - name: GITLAB_AUTH_TOKEN
          valueFrom:
            secretKeyRef:
              name: mcp-secrets
              key: gitlab-token
              optional: true
        - name: API_KEY
          valueFrom:
            secretKeyRef:
              name: mcp-secrets
              key: api-key
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: config.yaml
      volumes:
      - name: config
        configMap:
          name: mcp-config
---
apiVersion: v1
kind: Service
metadata:
  name: kubernetes-mcp-server
  namespace: mcp-system
spec:
  selector:
    app: kubernetes-mcp-server
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mcp-service-account
  namespace: mcp-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mcp-cluster-role
rules:
- apiGroups: [""]
  resources: ["pods", "services", "events", "configmaps", "secrets", "namespaces", "nodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mcp-role-binding
subjects:
- kind: ServiceAccount
  name: mcp-service-account
  namespace: mcp-system
roleRef:
  kind: ClusterRole
  name: mcp-cluster-role
  apiGroup: rbac.authorization.k8s.io
```

Apply the configuration:

```bash
kubectl apply -f deployment.yaml
```

### Step 5: Access the Server

Create an Ingress or port-forward to access the server:

```bash
kubectl port-forward -n mcp-system svc/kubernetes-mcp-server 8080:80
```

## Helm Chart

For Kubernetes users, the Helm chart provides the easiest installation method.

### Step 1: Add the Helm Repository

```bash
helm repo add blankcut https://blankcut.github.io/helm-charts
helm repo update
```

### Step 2: Configure Values

Create a `values.yaml` file:

```yaml
image:
  repository: blankcut/kubernetes-mcp-server
  tag: latest

config:
  server:
    address: ":8080"
  kubernetes:
    inCluster: true
    defaultNamespace: "default"
  argocd:
    url: "https://argocd.example.com"
  gitlab:
    url: "https://gitlab.com"
  claude:
    modelID: "claude-3-haiku-20240307"

secrets:
  claude:
    apiKey: "your_claude_api_key"
  argocd:
    username: "your_argocd_username"
    password: "your_argocd_password"
  gitlab:
    authToken: "your_gitlab_token"

service:
  type: ClusterIP

ingress:
  enabled: false
  # Uncomment to enable ingress
  # hosts:
  #   - host: mcp.example.com
  #     paths:
  #       - path: /
  #         pathType: Prefix
```

### Step 3: Install the Chart

```bash
helm install kubernetes-mcp-server blankcut/kubernetes-claude-mcp -f values.yaml -n mcp-system
```

### Step 4: Verify the Installation

```bash
kubectl get pods -n mcp-system
```

## Manual Binary

For environments where Docker or Kubernetes is not available, you can run the binary directly.

### Step 1: Download the Latest Release

Visit the [Releases page](https://github.com/blankcut/kubernetes-mcp-server/releases) and download the appropriate binary for your platform.

### Step 2: Make the Binary Executable

```bash
chmod +x mcp-server
```

### Step 3: Create Configuration File

Create a `config.yaml` file in the same directory:

```yaml
server:
  address: ":8080"
  readTimeout: 30
  writeTimeout: 60
  auth:
    apiKey: "your_api_key_for_server_access"

kubernetes:
  kubeconfig: "/path/to/.kube/config"  # Path to your kubeconfig file
  inCluster: false
  defaultContext: ""
  defaultNamespace: "default"

argocd:
  url: "https://argocd.example.com"
  username: "your_argocd_username"
  password: "your_argocd_password"
  insecure: true

gitlab:
  url: "https://gitlab.com"
  authToken: "your_gitlab_token"
  apiVersion: "v4"
  projectPath: ""

claude:
  apiKey: "your_claude_api_key"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-sonnet-4.5-20250514"
  maxTokens: 8192
  temperature: 0.3
```

### Step 4: Run the Server

```bash
export CLAUDE_API_KEY=your_claude_api_key
export API_KEY=your_api_key_for_server
./mcp-server --config config.yaml
```

## Verifying the Installation

To verify your installation is working correctly:

1. Check the health endpoint:

```bash
curl http://localhost:8080/api/v1/health
```

2. List Kubernetes namespaces:

```bash
curl -H "X-API-Key: your_api_key" http://localhost:8080/api/v1/namespaces
```

3. Test a resource query:

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your_api_key" \
  -d '{
    "action": "queryResource",
    "resource": "pod",
    "name": "example-pod",
    "namespace": "default",
    "query": "Is this pod healthy?"
  }' \
  http://localhost:8080/api/v1/mcp/resource
```

## Security Considerations

When deploying Kubernetes Claude MCP, consider the following security best practices:

1. **API Access**: Use a strong API key and restrict access to the server.
2. **Kubernetes Permissions**: Use a service account with the minimum required permissions.
3. **Secrets Management**: Store credentials in Kubernetes Secrets or a secure vault.
4. **Network Isolation**: Consider network policies to limit access to the server.
5. **TLS**: Use TLS to encrypt connections to the server.

For more security recommendations, see the [Security Best Practices](/docs/security-best-practices) guide.

## Troubleshooting

If you encounter issues during installation, check:

1. **Logs**: View server logs for error messages
   ```bash
   # For Docker Compose
   docker-compose logs
   
   # For Kubernetes
   kubectl logs -n mcp-system deployment/kubernetes-mcp-server
   ```

2. **Configuration**: Verify your `config.yaml` has the correct settings
3. **Connectivity**: Ensure the server can connect to Kubernetes, ArgoCD, and GitLab
4. **API Key**: Verify you're using the correct API key in requests

For more troubleshooting tips, see the [Troubleshooting](/docs/troubleshooting-resources) guide.

## Next Steps

After successful installation, continue with:

- [Configuration Guide](/docs/configuration) - Configure the server for your environment
- [API Reference](/docs/api-overview) - Explore the API endpoints
- [Examples](/docs/examples/basic-usage) - See examples of common use cases