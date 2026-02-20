# Quick Start Guide

## 1. Configure GitHub Secrets (REQUIRED)

Before anything will work, add these secrets to your GitHub repository:

1. Go to: https://github.com/Blankcut/kubernetes-mcp-server/settings/secrets/actions
2. Click "New repository secret"
3. Add:
   - **Name**: `DOCKERHUB_USERNAME` | **Value**: `your-dockerhub-username`
   - **Name**: `DOCKERHUB_TOKEN` | **Value**: `your-dockerhub-token` (Get from https://hub.docker.com/settings/security)

## 2. Test the CI Pipeline

```bash
# Commit and push the new workflows
git add .github/ kubernetes-claude-mcp/.golangci.yml
git commit -m "Add CI/CD workflows with multi-arch Docker support"
git push origin main
```

Watch the build at: https://github.com/Blankcut/kubernetes-mcp-server/actions

## 3. Create Your First Release

```bash
# Tag the release
git tag -a v0.1.0 -m "Initial release"
git push origin v0.1.0
```

This will:
- Build multi-arch Docker images (AMD64 + ARM64)
- Push to Docker Hub: `blankcut/kubernetes-mcp-server:v0.1.0`
- Create GitHub Release with binaries

## 4. Test the Docker Image

```bash
# Pull and run
docker pull blankcut/kubernetes-mcp-server:latest
docker run --rm blankcut/kubernetes-mcp-server:latest --help
```

## What You Get

### CI Workflow (on every push/PR)
- ✅ Code linting
- ✅ Tests with coverage
- ✅ Multi-platform builds
- ✅ Security scanning

### Release Workflow (on version tags)
- ✅ Multi-arch Docker images (AMD64 + ARM64)
- ✅ Docker Hub publishing
- ✅ GitHub Release with binaries
- ✅ SBOM generation
- ✅ Vulnerability scanning

## Docker Image Tags

After release, your images will be available as:
- `blankcut/kubernetes-mcp-server:v0.1.0` (specific version)
- `blankcut/kubernetes-mcp-server:0.1` (major.minor)
- `blankcut/kubernetes-mcp-server:0` (major)
- `blankcut/kubernetes-mcp-server:latest` (latest release)

## Supported Architectures

- `linux/amd64` (Intel/AMD)
- `linux/arm64` (ARM/Apple Silicon)

Docker automatically pulls the right one for your system!

## Troubleshooting

**Workflow fails with auth error?**
→ Check that GitHub secrets are set correctly

**Tests fail?**
→ Run locally first: `cd kubernetes-claude-mcp && go test ./...`

**Linting fails?**
→ Run locally: `cd kubernetes-claude-mcp && golangci-lint run`

## Next Steps

1. Add badges to README (see `.github/README_BADGES.md`)
2. Create Helm chart for Kubernetes deployment
3. Add more tests
4. Set up branch protection rules

## Resources

- Full setup guide: `.github/SETUP.md`
- Complete documentation: `CICD_SETUP_COMPLETE.md`
- Docker Hub: https://hub.docker.com/r/blankcut/kubernetes-mcp-server

