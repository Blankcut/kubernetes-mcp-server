# Kubernetes MCP Server - Postman Collection

This directory contains a comprehensive Postman collection for testing all Kubernetes MCP Server API endpoints.

## Quick Start

### 1. Import the Collection

1. Open Postman
2. Click **Import** button
3. Select the file: `Kubernetes-MCP-Server-API.postman_collection.json`
4. The collection will be imported with all endpoints organized by category

### 2. Configure Variables

After importing, configure the collection variables:

1. Click on the collection name in Postman
2. Go to the **Variables** tab
3. Update the following variables:

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `base_url` | API server base URL | `http://localhost:8080` or `http://mcp-server.blankcut.com` |
| `api_key` | Your API key for authentication | Get from your config file or environment |

**For local testing with port-forward:**
```bash
kubectl port-forward -n kubernetes-mcp-server-ns svc/kubernetes-mcp-server-release 8080:8080
```
Then set `base_url` to `http://localhost:8080`

**For production:**
Set `base_url` to `http://mcp-server.blankcut.com` (once DNS is configured)

### 3. Get Your API Key

The API key is configured in your server's config file or environment variable:

**From config file:**
```bash
# Check the config file
cat kubernetes-claude-mcp/config.yaml | grep apiKey
```

**From Kubernetes secret (production):**
```bash
kubectl get secret -n kubernetes-mcp-server-ns kubernetes-mcp-server-release-secret \
  -o jsonpath='{.data.API_KEY}' | base64 -d
```

**From environment variable:**
```bash
echo $API_KEY
```

## Collection Structure

The collection is organized into the following categories:

### 1. Health Endpoints (No Auth Required)
- **Health Check** - Overall health status of all services
- **Liveness Probe** - Kubernetes liveness check
- **Readiness Probe** - Kubernetes readiness check

### 2. Kubernetes Resources (Auth Required)
- **List Namespaces** - Get all namespaces
- **List Resources** - List resources by type (pods, deployments, services, etc.)
- **Get Resource** - Get specific resource details
- **Get Events** - Get Kubernetes events

### 3. Namespace Analysis (Auth Required)
- **Get Namespace Topology** - Topology view of namespace resources
- **Get Namespace Graph** - Graph representation of resource relationships
- **Get Namespace Resources** - All resources in a namespace
- **Get Namespace Analysis** - AI-powered namespace analysis

### 4. ArgoCD (Auth Required)
- **List Applications** - Get all ArgoCD applications
- **Get Application** - Get specific application details

### 5. GitLab (Auth Required)
- **List Projects** - Get all accessible GitLab projects
- **List Pipelines** - Get pipelines for a project

### 6. MCP - Claude AI (Auth Required)
- **Generic MCP Request** - General AI analysis
- **Query Resource** - AI analysis of specific Kubernetes resource
- **Troubleshoot Resource** - AI-powered troubleshooting
- **Analyze Commit** - AI analysis of GitLab commits
- **Analyze Merge Request** - AI review of merge requests

## Authentication

The collection uses API Key authentication with two supported methods:

### Method 1: X-API-Key Header (Default)
The collection is pre-configured to use this method:
```
X-API-Key: your-api-key-here
```

### Method 2: Authorization Bearer Token
You can also use Bearer token authentication by modifying individual requests:
```
Authorization: Bearer your-api-key-here
```

## Testing Examples

### Example 1: Check Health
```
GET {{base_url}}/api/v1/health
```
No authentication required. Should return status of all services.

### Example 2: List Namespaces
```
GET {{base_url}}/api/v1/namespaces
X-API-Key: {{api_key}}
```
Returns all namespaces in the cluster.

### Example 3: Troubleshoot a Pod
```
POST {{base_url}}/api/v1/mcp/troubleshoot
X-API-Key: {{api_key}}
Content-Type: application/json

{
  "resource": "pod",
  "name": "my-pod",
  "namespace": "default",
  "query": "Why is this pod crashing?"
}
```
Returns AI-powered troubleshooting analysis.

## Common Query Parameters

### Kubernetes Resources
- `namespace` - Filter by namespace (optional for cluster-scoped resources)
- `resource` - Resource type (pods, deployments, services, etc.)
- `name` - Resource name

### Events
- `namespace` - Namespace to filter events
- `resource` - Resource type to filter events
- `name` - Resource name to filter events

## Response Formats

All endpoints return JSON responses:

### Success Response
```json
{
  "status": "success",
  "data": { ... }
}
```

### Error Response
```json
{
  "error": "Error message",
  "details": "Additional error details"
}
```

## Troubleshooting

### Authentication Errors
If you get `{"error":"Authentication required"}`:
1. Verify your API key is correct
2. Check that the `api_key` variable is set in the collection
3. Ensure the API key matches the server configuration

### Connection Errors
If you get connection refused:
1. Verify the server is running
2. Check the `base_url` variable
3. If using port-forward, ensure it's active
4. Check firewall/network settings

### 404 Errors
If you get 404 errors:
1. Verify the endpoint path is correct
2. Check API version (should be `/api/v1`)
3. Ensure the server version matches the collection

## Additional Resources

- [API Documentation](../docs/src/content/docs/api-overview.md)
- [Server Configuration](../docs/src/content/docs/configuration.md)
- [GitHub Repository](https://github.com/Blankcut/kubernetes-mcp-server)

