# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Optional Workload Identity Federation for Claude authentication. The server can
  exchange a short-lived OIDC token from your own identity provider for an Anthropic
  access token instead of holding a long-lived API key. Opt-in and fully backwards
  compatible: omit the new `claude.federation` block and the existing `apiKey` path
  is unchanged. Configurable via YAML or the same environment variables the official
  Anthropic SDKs read.
- GitHub Actions CI/CD workflows with multi-architecture Docker support (AMD64 + ARM64)
- golangci-lint configuration for code quality
- Trivy security scanning in CI/CD pipeline
- Codecov integration for test coverage reporting
- Docker Hub publishing automation
- Multi-architecture Docker image builds
- SBOM (Software Bill of Materials) generation
- Comprehensive CI/CD documentation
- Quick start guides for CI/CD setup
- Kubernetes liveness probe endpoint (`/api/v1/health/live`)
- Kubernetes readiness probe endpoint (`/api/v1/health/ready`)
- Enhanced configuration validation on startup
- CHANGELOG.md for tracking version history
- Helm chart for Kubernetes deployment with production-ready defaults
- Optional HorizontalPodAutoscaler (HPA) support via Helm values
- Optional Ingress support via Helm values
- Comprehensive RBAC configuration with ClusterRole support
- GitHub issue templates for bug reports and feature requests
- Pull request template for standardized contributions
- Dependabot configuration for automated dependency updates
- README badges for CI status, Docker Hub, Go Report Card, and more
- Astro-based documentation site with modern UI
- Docker image for documentation site (`blankcut/kubernetes-mcp-server-docs`)
- GitHub Actions workflow for building and publishing docs Docker image
- Helm chart for deploying documentation site to Kubernetes (in `tmp/` directory)
- nginx configuration for serving static documentation site
- Multi-architecture support for docs Docker image (AMD64 + ARM64)

### Changed
- Updated Claude model to Sonnet 4.5 (claude-sonnet-4.5-20250514) across all documentation
- Updated default maxTokens to 8192 for better responses
- Updated default temperature to 0.3 for more focused responses
- Improved configuration examples in documentation
- Enhanced config validation to check all required fields and value ranges
- Simplified deployment approach to use Helm chart as primary method
- Updated documentation site domain from `kubernetes-mcp-server.dropasite.com` to `kubernetes-mcp-server.blankcut.com`

### Removed
- Raw Kubernetes manifests (k8s/ directory) in favor of Helm chart only

### Fixed
- Log key/value pairs are structured fields again. `Debug`, `Info`, `Warn`,
  `Error` and `Fatal` on the project logger ran their arguments through
  `fmt.Sprint`, so every call site's pairs were glued onto the message
  (`"msg":"HTTP requestmethodGETpath/api/v1/argocd/applicationsstatus200"`).
  Fixed once in `pkg/logging`; no call sites changed.
- The ArgoCD client treats a `401` as an error. It used to hand 401 responses
  back as successes, so a revoked or expired token made list and get calls
  decode the error body into empty results, and the refresh-on-401 path could
  never run. Every status outside 2xx from ArgoCD and GitLab is now a typed
  error carrying the status code. With username/password credentials a 401
  creates a new session and retries the request once. A 401 from
  `GET /api/v1/argocd/applications` or `/argocd/applications/{name}` answers
  `503` with the same message as the authorization probe, even before the
  first probe has run; the single-application endpoint used to answer `200`
  with an empty application.
- The ArgoCD and GitLab clients no longer retry permanent failures. Every
  error, including 401, 403 and 404, was retried with a 1s+2s backoff, adding
  three seconds to an answer that could not change. Only transport errors,
  `429` and `5xx` are retried now, and transport errors and `5xx` only for
  idempotent methods, so a retried POST cannot create a merge request comment
  twice. A retried request re-sends its full body (it used to send an empty
  one), and cancelling the context ends a backoff wait immediately.
- `GET /api/v1/argocd/applications` no longer answers an ArgoCD RBAC denial
  with `200` and an empty list. ArgoCD filters out applications the caller
  may not `get`, so on 2026-09-28 a token that had lost its grant looked
  exactly like a cluster with no applications for ~16 hours, while every
  health check stayed green. The server now probes
  `/api/v1/account/can-i/applications/get/*/*` at startup and every 60s,
  logs at ERROR when the token is denied or rejected, reports the result as
  `components.argocd` in `/api/v1/health` and `/api/v1/health/ready`, and
  returns `503` for an empty list while the probe says the token was refused.
  Readiness is deliberately not failed by this; its status reads `degraded`.
- The credential provider no longer fails startup when only
  `claude.federation` is configured. Fixing `Validate()` alone was not
  enough: `LoadCredentials` runs earlier and had its own hard requirement
  for a static key, so a federated deployment still crash-looped before the
  Claude client was ever constructed.
- Configuration validation no longer requires `claude.apiKey` when a complete
  `claude.federation` block is present. Previously a deployment that finished
  moving to Workload Identity Federation and removed its static key would fail
  startup validation and crash-loop, despite being correctly configured.
- Configuration file security (config.yaml.example created with placeholders)

## [0.1.0] - TBD

### Added
- Initial release
- Kubernetes cluster management via MCP protocol
- ArgoCD GitOps integration
- GitLab CI/CD integration
- Claude AI integration for intelligent troubleshooting
- Resource correlation and analysis
- Health check endpoints
- Configurable logging
- Docker support with multi-stage builds
- Non-root container security

### Security
- Non-root user in Docker container
- Secure credential management via environment variables
- TLS support for external connections

## Release Notes

### Upcoming v0.1.0

This is the initial public release of the Kubernetes MCP Server. Key features include:

- **MCP Protocol Integration**: Full support for Model Context Protocol with Claude AI
- **Kubernetes Management**: Comprehensive Kubernetes cluster management capabilities
- **GitOps Integration**: Native ArgoCD and GitLab integration
- **Intelligent Troubleshooting**: AI-powered resource correlation and analysis
- **Multi-Architecture Support**: Docker images for AMD64 and ARM64
- **Production Ready**: Security scanning, health checks, and comprehensive logging

### Migration Guide

This is the first release, so no migration is needed.

### Breaking Changes

None in this release.

### Deprecations

None in this release.

---

## Version History

- **Unreleased**: Current development version
- **0.1.0**: Initial public release (upcoming)

## Contributing

When contributing to this project, please update this CHANGELOG.md file with your changes under the "Unreleased" section. Follow the format:

- **Added** for new features
- **Changed** for changes in existing functionality
- **Deprecated** for soon-to-be removed features
- **Removed** for now removed features
- **Fixed** for any bug fixes
- **Security** for vulnerability fixes

## Links

- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [GitHub Releases](https://github.com/Blankcut/kubernetes-mcp-server/releases)

