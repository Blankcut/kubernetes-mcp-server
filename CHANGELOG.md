# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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

