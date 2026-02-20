# Contributing to Kubernetes MCP Server

Thank you for your interest in contributing to Kubernetes MCP Server! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct (see CODE_OF_CONDUCT.md).

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check the existing issues to avoid duplicates. When creating a bug report, include as many details as possible:

- Use a clear and descriptive title
- Describe the exact steps to reproduce the problem
- Provide specific examples to demonstrate the steps
- Describe the behavior you observed and what you expected
- Include logs, screenshots, and configuration files (remove sensitive data)
- Specify your environment (Kubernetes version, deployment method, etc.)

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion:

- Use a clear and descriptive title
- Provide a detailed description of the proposed functionality
- Explain why this enhancement would be useful
- List any alternative solutions you've considered

### Pull Requests

1. Fork the repository and create your branch from `main`
2. If you've added code that should be tested, add tests
3. Ensure the test suite passes
4. Make sure your code follows the existing style
5. Write a clear commit message
6. Update documentation as needed

## Development Setup

### Prerequisites

- Go 1.21 or higher
- Docker
- Kubernetes cluster (for testing)
- kubectl configured

### Local Development

1. Clone the repository:
```bash
git clone https://github.com/Blankcut/kubernetes-mcp-server.git
cd kubernetes-mcp-server
```

2. Install dependencies:
```bash
cd kubernetes-claude-mcp
go mod download
```

3. Copy the example config:
```bash
cp config.yaml.example config.yaml
# Edit config.yaml with your settings
```

4. Run the server:
```bash
go run ./cmd/server/main.go
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test ./internal/k8s/...
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` to format your code
- Run `golangci-lint` before submitting
- Write meaningful commit messages

### Commit Messages

- Use the present tense ("Add feature" not "Added feature")
- Use the imperative mood ("Move cursor to..." not "Moves cursor to...")
- Limit the first line to 72 characters or less
- Reference issues and pull requests liberally after the first line

## Project Structure

```
kubernetes-mcp-server/
├── kubernetes-claude-mcp/
│   ├── cmd/              # Application entry points
│   ├── internal/         # Private application code
│   ├── pkg/              # Public libraries
│   ├── tests/            # Test files
│   └── config.yaml       # Configuration
├── helm/                 # Helm charts
├── k8s/                  # Kubernetes manifests
└── docs/                 # Documentation
```

## Release Process

Releases are automated through GitHub Actions. To create a release:

1. Update CHANGELOG.md
2. Create and push a tag: `git tag -a v0.1.0 -m "Release v0.1.0"`
3. Push the tag: `git push origin v0.1.0`
4. GitHub Actions will build and publish the release

## Questions?

Feel free to open an issue with your question or reach out to the maintainers.

Thank you for contributing!

