// internal/correlator/helm_correlator.go

package correlator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/gitlab"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/helm"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// HelmCorrelator correlates Helm charts with Kubernetes resources
type HelmCorrelator struct {
	gitlabClient *gitlab.Client
	helmParser   *helm.Parser
	logger       *logging.Logger
}

// NewHelmCorrelator creates a new Helm correlator
func NewHelmCorrelator(gitlabClient *gitlab.Client, logger *logging.Logger) *HelmCorrelator {
	if logger == nil {
		logger = logging.NewLogger().Named("helm-correlator")
	}

	return &HelmCorrelator{
		gitlabClient: gitlabClient,
		helmParser:   helm.NewParser(logger.Named("helm")),
		logger:       logger,
	}
}

// AnalyzeCommitHelmChanges analyzes Helm changes in a commit
func (c *HelmCorrelator) AnalyzeCommitHelmChanges(ctx context.Context, projectID string, commitSHA string) ([]string, error) {
	c.logger.Debug("Analyzing Helm changes in commit", "projectID", projectID, "commitSHA", commitSHA)

	// Get commit diff
	diffs, err := c.gitlabClient.GetCommitDiff(ctx, projectID, commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit diff: %w", err)
	}

	// Identify Helm chart changes
	helmCharts := c.identifyHelmCharts(diffs)
	if len(helmCharts) == 0 {
		c.logger.Debug("No Helm chart changes found in commit")
		return nil, nil
	}

	// Analyze each chart
	var affectedResources []string

	for chartPath, files := range helmCharts {
		resources, err := c.analyzeHelmChart(ctx, projectID, commitSHA, chartPath, files)
		if err != nil {
			c.logger.Warn("Failed to analyze Helm chart", "chartPath", chartPath, "error", err)
			continue
		}

		affectedResources = append(affectedResources, resources...)
	}

	return affectedResources, nil
}

// AnalyzeMergeRequestHelmChanges analyzes Helm changes in a merge request
func (c *HelmCorrelator) AnalyzeMergeRequestHelmChanges(ctx context.Context, projectID string, mergeRequestIID int) ([]string, error) {
	c.logger.Debug("Analyzing Helm changes in merge request", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	// Get merge request changes
	mrChanges, err := c.gitlabClient.GetMergeRequestChanges(ctx, projectID, mergeRequestIID)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request changes: %w", err)
	}

	// Identify Helm chart changes
	var gitlabDiffs []models.GitLabDiff
	for _, change := range mrChanges.Changes {
		diff := models.GitLabDiff{
			OldPath:     change.OldPath,
			NewPath:     change.NewPath,
			Diff:        change.Diff,
			NewFile:     change.NewFile,
			RenamedFile: change.RenamedFile,
			DeletedFile: change.DeletedFile,
		}
		gitlabDiffs = append(gitlabDiffs, diff)
	}
	helmCharts := c.identifyHelmCharts(gitlabDiffs)
	if len(helmCharts) == 0 {
		c.logger.Debug("No Helm chart changes found in merge request")
		return nil, nil
	}

	// Get commits in the merge request
	commits, err := c.gitlabClient.GetMergeRequestCommits(ctx, projectID, mergeRequestIID)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request commits: %w", err)
	}

	// Use the latest commit SHA for analysis
	var latestCommitSHA string
	if len(commits) > 0 {
		latestCommitSHA = commits[0].ID
	} else {
		latestCommitSHA = mrChanges.DiffRefs.HeadSHA
	}

	// Analyze each chart
	var affectedResources []string

	for chartPath, files := range helmCharts {
		resources, err := c.analyzeHelmChart(ctx, projectID, latestCommitSHA, chartPath, files)
		if err != nil {
			c.logger.Warn("Failed to analyze Helm chart", "chartPath", chartPath, "error", err)
			continue
		}

		affectedResources = append(affectedResources, resources...)
	}

	return affectedResources, nil
}

// identifyHelmCharts identifies Helm charts in changed files
func (c *HelmCorrelator) identifyHelmCharts(diffs []models.GitLabDiff) map[string][]string {
	helmCharts := make(map[string][]string)

	for _, diff := range diffs {
		path := diff.NewPath

		// Skip deleted files
		if diff.DeletedFile {
			continue
		}

		// Check if it's a Helm-related file
		if strings.Contains(path, "Chart.yaml") ||
			strings.Contains(path, "values.yaml") ||
			(strings.Contains(path, "templates/") && strings.HasSuffix(path, ".yaml")) {

			// Extract chart path (parent directory of Chart.yaml or parent's parent for templates)
			chartPath := filepath.Dir(path)
			if strings.Contains(path, "templates/") {
				chartPath = filepath.Dir(filepath.Dir(path))
			}

			// Add to chart files
			if _, exists := helmCharts[chartPath]; !exists {
				helmCharts[chartPath] = []string{}
			}

			helmCharts[chartPath] = append(helmCharts[chartPath], path)
		}
	}

	return helmCharts
}

// analyzeHelmChart analyzes changes in a Helm chart
func (c *HelmCorrelator) analyzeHelmChart(ctx context.Context, projectID, commitSHA, chartPath string, changedFiles []string) ([]string, error) {
	c.logger.Debug("Analyzing Helm chart", "chartPath", chartPath, "changedFiles", changedFiles)

	// Determine chart structure
	chartFiles := make(map[string]string)

	// Get Chart.yaml
	chartYaml, err := c.gitlabClient.GetFileContent(ctx, projectID, fmt.Sprintf("%s/Chart.yaml", chartPath), commitSHA)
	if err != nil {
		c.logger.Warn("Failed to get Chart.yaml", "error", err)
		// Try to continue without Chart.yaml
	} else {
		chartFiles["Chart.yaml"] = chartYaml
	}

	// Get values.yaml
	valuesYaml, err := c.gitlabClient.GetFileContent(ctx, projectID, fmt.Sprintf("%s/values.yaml", chartPath), commitSHA)

	if err != nil {
		c.logger.Warn("Failed to get values.yaml", "error", err)
		// Try to continue without values.yaml
	} else {
		chartFiles["values.yaml"] = valuesYaml
	}

	// Get template files
	for _, file := range changedFiles {
		if strings.Contains(file, "templates/") {
			content, fileErr := c.gitlabClient.GetFileContent(ctx, projectID, file, commitSHA)
			if fileErr != nil {
				c.logger.Warn("Failed to get template file", "file", file, "error", fileErr)
				continue
			}

			// Store template file relative to chart path
			relPath := strings.TrimPrefix(file, chartPath+"/")
			chartFiles[relPath] = content
		}
	}

	// Write chart files to disk for processing
	chartDir, err := c.helmParser.WriteChartFiles(chartFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to write chart files: %w", err)
	}

	// Parse chart to get manifests
	manifests, err := c.helmParser.ParseChart(ctx, chartDir, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chart: %w", err)
	}

	// Extract resources from manifests
	var resources []string
	for _, manifest := range manifests {
		// Extract resource information
		kind, name, namespace := c.extractResourceInfo(manifest)
		if kind != "" && name != "" {
			resource := fmt.Sprintf("%s/%s", kind, name)
			if namespace != "" {
				resource = fmt.Sprintf("%s/%s/%s", namespace, kind, name)
			}
			resources = append(resources, resource)
		}
	}

	c.logger.Debug("Analyzed Helm chart", "chartPath", chartPath, "resourceCount", len(resources))
	return resources, nil
}

// extractResourceInfo extracts kind, name, and namespace from a YAML manifest
func (c *HelmCorrelator) extractResourceInfo(manifest string) (kind, name, namespace string) {
	// Simple parsing - in a real implementation, use proper YAML parsing
	lines := strings.Split(manifest, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "kind:") {
			kind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
		} else if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "namespace:") {
			namespace = strings.TrimSpace(strings.TrimPrefix(line, "namespace:"))
		}
	}

	return kind, name, namespace
}

// Cleanup cleans up temporary resources
func (c *HelmCorrelator) Cleanup() {
	if c.helmParser != nil {
		c.helmParser.Cleanup()
	}
}
