

<p align="center">
  <a href="https://docs.blankcut.com/kubernetes-mcp-server/">
    <img src="assets/blankcut-logo.png" alt="Blank Cut" width="320">
  </a>
</p>



# Claude Kubernetes MCP Server

[![CI](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/ci.yml)
[![Release](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/release.yml/badge.svg)](https://github.com/Blankcut/kubernetes-mcp-server/actions/workflows/release.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/blankcut/kubernetes-mcp-server)](https://hub.docker.com/r/blankcut/kubernetes-mcp-server)
[![Docker Image Size](https://img.shields.io/docker/image-size/blankcut/kubernetes-mcp-server/latest)](https://hub.docker.com/r/blankcut/kubernetes-mcp-server)
[![Go Report Card](https://goreportcard.com/badge/github.com/Blankcut/kubernetes-mcp-server)](https://goreportcard.com/report/github.com/Blankcut/kubernetes-mcp-server)
[![License](https://img.shields.io/github/license/Blankcut/kubernetes-mcp-server)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/Blankcut/kubernetes-mcp-server)](https://github.com/Blankcut/kubernetes-mcp-server/releases)

> A Go server that answers questions about a Kubernetes cluster with Claude, using context from the
> Kubernetes API and, optionally, ArgoCD and GitLab.

**Documentation: [docs.blankcut.com/kubernetes-mcp-server](https://docs.blankcut.com/kubernetes-mcp-server/)**
— quick start, installation, every config key and endpoint, and troubleshooting.

## What it does

- **Reads the cluster** — namespaces, any resource type (custom resources too), events, and a
  per-namespace map of resources, relationships and health.
- **Finds problems without Claude** — Warning events and failing CronJobs (with the end of the
  failed run's log) become a list of issues in seconds (`?narrative=false`).
- **Asks Claude** about a resource, a whole namespace, or a GitLab merge request, with the context
  it collected.
- **Watches its own ArgoCD access** — it checks every 60 seconds that its token can still read
  Applications, so a lost RBAC grant shows up as an error rather than an empty list.
- **Authenticates to Claude with an API key or Workload Identity Federation**, so it can run
  without a long-lived key.

It only reads: nothing it does changes Kubernetes, ArgoCD or GitLab.

It is a **REST API**. Despite the name it does not implement the Model Context Protocol's JSON-RPC
transport, so MCP clients such as Claude Desktop can't connect to it directly; call it over HTTP.

## Quick start

Download a release binary (Linux and macOS, amd64 and arm64) from
[Releases](https://github.com/Blankcut/kubernetes-mcp-server/releases), write a `config.yaml`:

```yaml
server:
  address: ":8080"
  writeTimeout: 300
  auth:
    apiKey: "${API_KEY}"
kubernetes:
  inCluster: false          # uses $KUBECONFIG or ~/.kube/config
claude:
  apiKey: "${CLAUDE_API_KEY}"
  baseURL: "https://api.anthropic.com"
  modelID: "claude-sonnet-5"
  maxTokens: 8192
```

and run it:

```bash
export API_KEY=$(openssl rand -hex 32) CLAUDE_API_KEY=sk-ant-...
./mcp-server-darwin-arm64 --config config.yaml

curl -s -H "X-API-Key: $API_KEY" \
  "http://localhost:8080/api/v1/namespaces/default/analysis?narrative=false"
```

The [quick start](https://docs.blankcut.com/kubernetes-mcp-server/quick-start/) walks through it,
including checksum verification.

## Install

| Method | |
|---|---|
| Helm | `helm install kubernetes-mcp-server ./helm/kubernetes-mcp-server --set secrets.apiKey=... --set secrets.claudeApiKey=...` from a checkout of a release tag. See [Installation](https://docs.blankcut.com/kubernetes-mcp-server/installation/). |
| Docker | [`blankcut/kubernetes-mcp-server`](https://hub.docker.com/r/blankcut/kubernetes-mcp-server) (`linux/amd64`, `linux/arm64`); mount a config at `/app/config.yaml`. |
| Binary | [Releases](https://github.com/Blankcut/kubernetes-mcp-server/releases), with `checksums.txt`. |
| Source | Go 1.26+: `cd kubernetes-claude-mcp && go build -o mcp-server ./cmd/server`. |

## Development

```bash
cd kubernetes-claude-mcp
go test ./...
go run ./cmd/server --config config.yaml --log-level debug
```

A Postman collection is in [`postman/`](postman/). Changes are listed in
[`CHANGELOG.md`](CHANGELOG.md); contributions are welcome — see [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Donation

Please contribute to our coffee fund to help us continue to do great things: [Buy Me Coffee](https://buymeacoffee.com/blankcut)

## License

[MIT](./LICENSE)
