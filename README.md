

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
- **Reports its model usage as OpenTelemetry spans**, if you give it an OTLP endpoint: one span per
  Claude call, with the model and the token counts. See [Report model usage](#report-model-usage).

It only reads: nothing it does changes Kubernetes, ArgoCD or GitLab.

It is a **REST API**, and since 1.5.0 also a **Model Context Protocol server**: an MCP client can
connect to `/mcp` and call its cluster reads as tools. See [Connect an MCP client](#connect-an-mcp-client).

## Connect an MCP client

The server speaks MCP over Streamable HTTP at `/mcp`, with the same API key as the REST API
(`X-API-Key`, or `Authorization: Bearer`). In Claude Code:

```bash
claude mcp add --transport http kubernetes https://your-server.example.com/mcp \
  --header "X-API-Key: $KUBERNETES_MCP_API_KEY"
```

Any client that supports Streamable HTTP with a custom header connects the same way. There is no
stdio transport yet, so a client that can only launch a local process (Claude Desktop without a
remote connector) can't use it directly.

| Tool | What it returns |
|---|---|
| `troubleshoot_resource` | One resource's state, events, related objects, deployment history, and the issues the server's own checks found. Start here when something is failing. |
| `list_namespaces` | The cluster's namespaces. |
| `list_resources` | Resources of one kind, each with a one-line status. |
| `get_resource` | One resource's full object. |
| `get_events` | Recent events in a namespace, optionally for one resource. |
| `namespace_topology` | What exists in a namespace, how it relates, and each resource's health. |
| `list_argocd_applications`, `get_argocd_application` | Argo CD applications and their sync and health status. |

Every tool only reads. Two things differ from the REST API:

- **No tool calls Claude.** The REST analysis endpoints send what they collect to a model and return
  its answer. An MCP client is a model, so it gets the collected facts and reasons for itself, and a
  tool call costs you nothing in model usage.
- **Secret values are never returned.** `get_resource` on a Secret keeps the key names and replaces
  each value. The chart's default role can't read Secrets at all; this covers a server you run with
  wider access.

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

## Report model usage

Each call the server makes to Claude can be reported as one OpenTelemetry span, in the
[GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/), to any OTLP/HTTP
receiver: an OpenTelemetry Collector, or a backend that takes OTLP directly. It is off until an
endpoint is set, with the standard variables:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318        # /v1/traces is appended
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer $OTLP_TOKEN" # if the receiver wants one
```

A span says which model was asked and which answered, how many tokens the call used
(`gen_ai.usage.input_tokens`, which includes cached tokens, `gen_ai.usage.output_tokens`, and the
cache read and write counts), how long it took, whether it failed (`error.type`), and which analysis
it was for (`kubernetes_mcp_server.analysis`: `resource`, `namespace`, `troubleshoot` or
`merge_request`). It never carries a prompt, a response, or anything read from the cluster.

| Variable | |
|---|---|
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | The full URL to send to, used as given. Wins over the next one. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | A base URL; `/v1/traces` is appended. |
| `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_TRACES_HEADERS` | `name=value` pairs, comma-separated, sent with every export. Values are never logged. |
| `OTEL_EXPORTER_OTLP_TIMEOUT`, `OTEL_EXPORTER_OTLP_TRACES_TIMEOUT` | Milliseconds for one export. Default `5000`. |
| `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | How the server describes itself. `service.name` defaults to `kubernetes-mcp-server`; in a pod, `k8s.namespace.name` and `k8s.pod.name` are filled in unless you set them. |
| `OTEL_SDK_DISABLED` | `true` turns reporting off whatever else is set. |

Reporting cannot slow or fail a request: spans are queued and sent in the background, and if the
receiver is down they are dropped, with at most one warning a minute in the log. Spans are sent as
OTLP/HTTP JSON; OTLP over gRPC (`OTEL_EXPORTER_OTLP_PROTOCOL=grpc`, port 4317) is not supported, so
point it at the receiver's HTTP port. MCP tool calls make no model call and report nothing.

With the Helm chart, set the variables through `extraEnv`; `values.yaml` shows how to take the
token from a Secret.

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
