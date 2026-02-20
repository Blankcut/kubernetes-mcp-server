# GitHub Actions Setup Guide

This guide will help you configure GitHub Actions for the Kubernetes MCP Server project.

## Prerequisites

- GitHub repository: `Blankcut/kubernetes-mcp-server`
- Docker Hub account: `blankcut`
- Docker Hub Personal Access Token (PAT)

## Step 1: Configure GitHub Secrets

You need to add the following secrets to your GitHub repository:

### Navigate to Repository Settings

1. Go to your repository: https://github.com/Blankcut/kubernetes-mcp-server
2. Click on **Settings** (top menu)
3. In the left sidebar, click **Secrets and variables** → **Actions**
4. Click **New repository secret**

### Add Docker Hub Credentials

#### Secret 1: DOCKERHUB_USERNAME
- **Name**: `DOCKERHUB_USERNAME`
- **Value**: `your-dockerhub-username`
- Click **Add secret**

#### Secret 2: DOCKERHUB_TOKEN
- **Name**: `DOCKERHUB_TOKEN`
- **Value**: `your-dockerhub-token` (Get from https://hub.docker.com/settings/security)
- Click **Add secret**

## Step 2: Verify Workflows

After adding the secrets, the workflows will be ready to run:

### CI Workflow (`.github/workflows/ci.yml`)
**Triggers on**:
- Push to `main` or `develop` branches
- Pull requests to `main` or `develop` branches

**What it does**:
- Lints code with golangci-lint
- Runs tests with coverage reporting
- Builds binaries for multiple platforms (Linux/macOS, AMD64/ARM64)
- Tests Docker multi-arch build (without pushing)
- Scans for security vulnerabilities

### Release Workflow (`.github/workflows/release.yml`)
**Triggers on**:
- Push of version tags (e.g., `v1.0.0`, `v1.2.3`)
- Manual workflow dispatch

**What it does**:
- Builds multi-architecture Docker images (AMD64 + ARM64)
- Pushes images to Docker Hub with multiple tags:
  - `blankcut/kubernetes-mcp-server:v1.0.0` (version tag)
  - `blankcut/kubernetes-mcp-server:1.0` (major.minor)
  - `blankcut/kubernetes-mcp-server:1` (major)
  - `blankcut/kubernetes-mcp-server:latest` (for main branch)
- Generates Software Bill of Materials (SBOM)
- Scans images for vulnerabilities
- Creates GitHub Release with:
  - Compiled binaries for Linux/macOS (AMD64/ARM64)
  - Checksums
  - Release notes

## Step 3: Test the CI Workflow

### Option A: Push to main branch
```bash
git add .
git commit -m "Add GitHub Actions workflows"
git push origin main
```

### Option B: Create a Pull Request
```bash
git checkout -b feature/github-actions
git add .
git commit -m "Add GitHub Actions workflows"
git push origin feature/github-actions
# Then create a PR on GitHub
```

## Step 4: Create Your First Release

### 1. Ensure your code is ready
Make sure all tests pass and the code is in a releasable state.

### 2. Create and push a version tag
```bash
# Create a tag (use semantic versioning)
git tag -a v0.1.0 -m "Initial release"

# Push the tag to GitHub
git push origin v0.1.0
```

### 3. Monitor the release workflow
1. Go to: https://github.com/Blankcut/kubernetes-mcp-server/actions
2. You should see the "Release" workflow running
3. Wait for it to complete (usually 5-10 minutes)

### 4. Verify the release
After the workflow completes:

1. **Check Docker Hub**: https://hub.docker.com/r/blankcut/kubernetes-mcp-server/tags
   - You should see tags: `v0.1.0`, `0.1`, `0`, `latest`

2. **Check GitHub Releases**: https://github.com/Blankcut/kubernetes-mcp-server/releases
   - You should see a new release with binaries

3. **Test pulling the image**:
   ```bash
   docker pull blankcut/kubernetes-mcp-server:v0.1.0
   docker pull blankcut/kubernetes-mcp-server:latest
   ```

## Step 5: Verify Multi-Architecture Support

Test that both AMD64 and ARM64 images work:

```bash
# Pull and inspect the image
docker pull blankcut/kubernetes-mcp-server:latest
docker inspect blankcut/kubernetes-mcp-server:latest | grep Architecture

# Run on your current architecture
docker run --rm blankcut/kubernetes-mcp-server:latest --help
```

## Troubleshooting

### Workflow fails with "Error: Username and password required"
- Verify that `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets are set correctly
- Check that the token hasn't expired

### Multi-arch build fails
- Ensure QEMU is set up correctly (handled automatically by the workflow)
- Check Docker Hub account has permissions to push

### Tests fail
- Run tests locally first: `cd kubernetes-claude-mcp && go test ./...`
- Fix any failing tests before pushing

### Linting fails
- Run linter locally: `cd kubernetes-claude-mcp && golangci-lint run`
- Fix linting issues before pushing

## Best Practices

1. **Semantic Versioning**: Use semantic versioning for tags (v1.0.0, v1.1.0, v2.0.0)
2. **Changelog**: Maintain a CHANGELOG.md file for release notes
3. **Testing**: Always test locally before pushing
4. **Security**: Regularly update dependencies and scan for vulnerabilities
5. **Documentation**: Update README.md with each release

## Next Steps

1. Set up branch protection rules for `main` branch
2. Require CI checks to pass before merging PRs
3. Set up Codecov for test coverage reporting
4. Create issue templates and PR templates
5. Set up Dependabot for automated dependency updates

## Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Docker Buildx Multi-platform](https://docs.docker.com/build/building/multi-platform/)
- [Semantic Versioning](https://semver.org/)
- [golangci-lint](https://golangci-lint.run/)

