package correlator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/argocd"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/gitlab"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// GitOpsCorrelator correlates data between Kubernetes, ArgoCD, and GitLab
type GitOpsCorrelator struct {
	k8sClient      *k8s.Client
	argoClient     *argocd.Client
	gitlabClient   *gitlab.Client
	helmCorrelator *HelmCorrelator
	logger         *logging.Logger
}

// NewGitOpsCorrelator creates a new GitOps correlator
func NewGitOpsCorrelator(k8sClient *k8s.Client, argoClient *argocd.Client, gitlabClient *gitlab.Client, logger *logging.Logger) *GitOpsCorrelator {
	if logger == nil {
		logger = logging.NewLogger().Named("correlator")
	}

	correlator := &GitOpsCorrelator{
		k8sClient:    k8sClient,
		argoClient:   argoClient,
		gitlabClient: gitlabClient,
		logger:       logger,
	}

	// Initialize the Helm correlator
	correlator.helmCorrelator = NewHelmCorrelator(gitlabClient, logger.Named("helm"))

	return correlator
}

// AnalyzeMergeRequest analyzes a GitLab merge request and identifies affected Kubernetes resources
func (c *GitOpsCorrelator) AnalyzeMergeRequest(
	ctx context.Context,
	projectID string,
	mergeRequestIID int,
) ([]models.ResourceContext, error) {
	c.logger.Info("Analyzing merge request", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	// Get merge request details
	mergeRequest, err := c.gitlabClient.AnalyzeMergeRequest(ctx, projectID, mergeRequestIID)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze merge request: %w", err)
	}

	// Check if the MR affects Helm charts or Kubernetes manifests
	if !mergeRequest.MergeRequestContext.HelmChartAffected && !mergeRequest.MergeRequestContext.KubernetesManifest {
		c.logger.Info("Merge request does not affect Kubernetes resources")
		return []models.ResourceContext{}, nil
	}

	// Get all ArgoCD applications
	argoApps, err := c.argoClient.ListApplications(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list ArgoCD applications: %w", err)
	}

	// Find the project path
	projectPath := fmt.Sprintf("%s", projectID)
	project, err := c.gitlabClient.GetProject(ctx, projectID)
	if err == nil && project != nil {
		projectPath = project.PathWithNamespace
	}

	// For Helm-affected MRs, analyze Helm changes
	var helmAffectedResources []string
	if mergeRequest.MergeRequestContext.HelmChartAffected {
		helmResources, err := c.helmCorrelator.AnalyzeMergeRequestHelmChanges(ctx, projectID, mergeRequestIID)
		if err != nil {
			c.logger.Warn("Failed to analyze Helm changes in MR", "error", err)
		} else if len(helmResources) > 0 {
			helmAffectedResources = helmResources
			c.logger.Info("Found resources affected by Helm changes in MR", "count", len(helmResources))
		}
	}

	// Identify potentially affected applications
	var affectedApps []models.ArgoApplication
	for _, app := range argoApps {
		if isAppSourcedFromProject(app, projectPath) {
			// For each file changed in the MR, check if it affects the app
			isAffected := false

			// Check if any changed file affects the app
			for _, file := range mergeRequest.MergeRequestContext.AffectedFiles {
				if isFileInAppSourcePath(app, file) {
					isAffected = true
					break
				}
			}

			// Check Helm-derived resources
			if !isAffected && len(helmAffectedResources) > 0 {
				if appContainsAnyResource(ctx, c.argoClient, app, helmAffectedResources) {
					isAffected = true
				}
			}

			if isAffected {
				affectedApps = append(affectedApps, app)
			}
		}
	}

	// For each affected app, identify the resources that would be affected
	var result []models.ResourceContext
	for _, app := range affectedApps {
		c.logger.Info("Found potentially affected ArgoCD application", "app", app.Name)

		// Get resources managed by this application
		tree, err := c.argoClient.GetResourceTree(ctx, app.Name)
		if err != nil {
			c.logger.Warn("Failed to get resource tree", "app", app.Name, "error", err)
			continue
		}

		// For each resource in the app, create a deployment info object
		for _, node := range tree.Nodes {
			// Skip non-Kubernetes resources or resources with no name/namespace
			if node.Kind == "" || node.Name == "" {
				continue
			}

			// Avoid unnecessary duplicates in the result
			if isResourceAlreadyInResults(result, node.Kind, node.Name, node.Namespace) {
				continue
			}

			// Trace the deployment for this resource
			resourceContext, err := c.TraceResourceDeployment(
				ctx,
				node.Namespace,
				node.Kind,
				node.Name,
			)
			if err != nil {
				c.logger.Warn("Failed to trace resource deployment",
					"kind", node.Kind,
					"name", node.Name,
					"namespace", node.Namespace,
					"error", err)
				continue
			}

			// Add source info
			resourceContext.RelatedResources = append(resourceContext.RelatedResources,
				fmt.Sprintf("MergeRequest/%d", mergeRequestIID))

			// Add to results
			result = append(result, resourceContext)
		}
	}

	// Add cleanup on exit
	defer func() {
		if c.helmCorrelator != nil {
			c.helmCorrelator.Cleanup()
		}
	}()

	c.logger.Info("Analysis of merge request completed",
		"projectID", projectID,
		"mergeRequestIID", mergeRequestIID,
		"resourceCount", len(result))

	return result, nil
}

// TraceResourceDeployment traces the deployment history of a Kubernetes resource through GitOps
func (c *GitOpsCorrelator) TraceResourceDeployment(
	ctx context.Context,
	namespace, kind, name string,
) (models.ResourceContext, error) {
	c.logger.Info("Tracing resource deployment", "kind", kind, "name", name, "namespace", namespace)

	resourceContext := models.ResourceContext{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}

	var errors []string

	// Get Kubernetes resource information with enhanced error handling
	resource, err := c.k8sClient.GetResource(ctx, kind, namespace, name)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get Kubernetes resource: %v", err)
		errors = append(errors, errMsg)
		c.logger.Warn(errMsg, "kind", kind, "name", name, "namespace", namespace)
	} else {
		resourceContext.APIVersion = resource.GetAPIVersion()
		c.logger.Debug("Retrieved Kubernetes resource", "apiVersion", resourceContext.APIVersion)

		// Get events related to this resource with better error handling
		events, err := c.k8sClient.GetResourceEvents(ctx, namespace, kind, name)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to get resource events: %v", err)
			errors = append(errors, errMsg)
			c.logger.Warn(errMsg, "kind", kind, "name", name, "namespace", namespace)
		} else {
			resourceContext.Events = events
			c.logger.Debug("Retrieved resource events", "eventCount", len(events))
		}

		// TODO: Add related resources discovery in future enhancement
	}

	// Find the ArgoCD application managing this resource with enhanced error handling
	argoApps, err := c.argoClient.FindApplicationsByResource(ctx, kind, name, namespace)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to find ArgoCD applications: %v", err)
		errors = append(errors, errMsg)
		c.logger.Warn(errMsg, "kind", kind, "name", name, "namespace", namespace)
	} else if len(argoApps) > 0 {
		// Use the first application that manages this resource
		app := argoApps[0]
		resourceContext.ArgoApplication = &app
		resourceContext.ArgoSyncStatus = app.Status.Sync.Status
		resourceContext.ArgoHealthStatus = app.Status.Health.Status

		c.logger.Debug("Found ArgoCD application",
			"appName", app.Name,
			"syncStatus", app.Status.Sync.Status,
			"healthStatus", app.Status.Health.Status)

		// Get recent syncs
		history, err := c.argoClient.GetApplicationHistory(ctx, app.Name)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to get application history: %v", err)
			errors = append(errors, errMsg)
			c.logger.Warn(errMsg)
		} else {
			// Limit to recent syncs (last 5)
			if len(history) > 5 {
				history = history[:5]
			}
			resourceContext.ArgoSyncHistory = history
		}

		// Connect to GitLab if we have source information
		if app.Spec.Source.RepoURL != "" {
			// Extract GitLab project path from repo URL
			projectPath := extractGitLabProjectPath(app.Spec.Source.RepoURL)
			if projectPath != "" {
				project, err := c.gitlabClient.GetProjectByPath(ctx, projectPath)
				if err != nil {
					errMsg := fmt.Sprintf("Failed to get GitLab project: %v", err)
					errors = append(errors, errMsg)
					c.logger.Warn(errMsg)
				} else {
					resourceContext.GitLabProject = project

					// Get recent pipelines
					pipelines, err := c.gitlabClient.ListPipelines(ctx, fmt.Sprintf("%d", project.ID))
					if err != nil {
						errMsg := fmt.Sprintf("Failed to list pipelines: %v", err)
						errors = append(errors, errMsg)
						c.logger.Warn(errMsg)
					} else {
						// Get the latest pipeline
						if len(pipelines) > 0 {
							resourceContext.LastPipeline = &pipelines[0]
						}
					}

					// Find environment from ArgoCD application
					environment := extractEnvironmentFromArgoApp(app)
					if environment != "" {
						// Get recent deployments to this environment
						deployments, err := c.gitlabClient.FindRecentDeployments(
							ctx,
							fmt.Sprintf("%d", project.ID),
							environment,
						)
						if err != nil {
							errMsg := fmt.Sprintf("Failed to find deployments: %v", err)
							errors = append(errors, errMsg)
							c.logger.Warn(errMsg)
						} else if len(deployments) > 0 {
							resourceContext.LastDeployment = &deployments[0]
						}
					}

					// Get recent commits
					sinceTime := time.Now().Add(-24 * time.Hour) // Last 24 hours
					commits, err := c.gitlabClient.FindRecentChanges(
						ctx,
						fmt.Sprintf("%d", project.ID),
						sinceTime,
					)
					if err != nil {
						errMsg := fmt.Sprintf("Failed to find recent changes: %v", err)
						errors = append(errors, errMsg)
						c.logger.Warn(errMsg)
					} else {
						// Here we'll limit to recent commits (last 5)...
						if len(commits) > 5 {
							commits = commits[:5]
						}
						resourceContext.RecentCommits = commits
					}
				}
			}
		}
	}

	// Collect any errors that occurred during correlation
	resourceContext.Errors = errors

	c.logger.Info("Resource deployment traced",
		"kind", kind,
		"name", name,
		"namespace", namespace,
		"argoApp", resourceContext.ArgoApplication != nil,
		"gitlabProject", resourceContext.GitLabProject != nil,
		"errors", len(errors))

	return resourceContext, nil
}

// isFileInAppSourcePath checks if a file is in the application's source path
func isFileInAppSourcePath(app models.ArgoApplication, file string) bool {
	sourcePath := app.Spec.Source.Path
	if sourcePath == "" {
		// If no specific path is provided, any file could affect the app
		return true
	}

	return strings.HasPrefix(file, sourcePath)
}

// hasHelmChanges checks if any of the changed files are related to Helm charts
//
//nolint:unused // Reserved for future Helm change detection
func hasHelmChanges(diffs []models.GitLabDiff) bool {
	for _, diff := range diffs {
		path := diff.NewPath

		if strings.Contains(path, "Chart.yaml") ||
			strings.Contains(path, "values.yaml") ||
			(strings.Contains(path, "templates/") && strings.HasSuffix(path, ".yaml")) {
			return true
		}
	}

	return false
}

// appContainsAnyResource checks if an ArgoCD application contains any of the specified resources
func appContainsAnyResource(ctx context.Context, argoClient *argocd.Client, app models.ArgoApplication, resources []string) bool {
	tree, err := argoClient.GetResourceTree(ctx, app.Name)
	if err != nil {
		return false
	}

	for _, resource := range resources {
		parts := strings.Split(resource, "/")

		if len(parts) == 2 {
			// Format: Kind/Name
			kind := parts[0]
			name := parts[1]

			for _, node := range tree.Nodes {
				if strings.EqualFold(node.Kind, kind) && node.Name == name {
					return true
				}
			}
		} else if len(parts) == 3 {
			// Format: Namespace/Kind/Name
			namespace := parts[0]
			kind := parts[1]
			name := parts[2]

			for _, node := range tree.Nodes {
				if strings.EqualFold(node.Kind, kind) && node.Name == name && node.Namespace == namespace {
					return true
				}
			}
		}
	}

	return false
}

// FindResourcesAffectedByCommit finds resources affected by a specific Git commit
func (c *GitOpsCorrelator) FindResourcesAffectedByCommit(
	ctx context.Context,
	projectID string,
	commitSHA string,
) ([]models.ResourceContext, error) {
	c.logger.Info("Finding resources affected by commit", "projectID", projectID, "commitSHA", commitSHA)

	var result []models.ResourceContext

	// Get commit information from GitLab
	commit, err := c.gitlabClient.GetCommit(ctx, projectID, commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}
	c.logger.Info("Processing commit", "author", commit.AuthorName, "message", commit.Title)

	// Get commit diff to see what files were changed
	diffs, err := c.gitlabClient.GetCommitDiff(ctx, projectID, commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit diff: %w", err)
	}

	// Get all ArgoCD applications
	argoApps, err := c.argoClient.ListApplications(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list ArgoCD applications: %w", err)
	}

	// Find applications that use this GitLab project as source
	projectPath := fmt.Sprintf("%s", projectID) // This might need more parsing depending on projectID format
	project, err := c.gitlabClient.GetProject(ctx, projectID)
	if err == nil && project != nil {
		projectPath = project.PathWithNamespace
	}

	// For each application, check if it's affected by the changed files
	for _, app := range argoApps {
		if !isAppSourcedFromProject(app, projectPath) {
			continue
		}

		// Check if the commit affects files used by this application
		if isAppAffectedByDiffs(app, diffs) {
			c.logger.Info("Found affected ArgoCD application", "app", app.Name)

			// Get resources managed by this application
			tree, err := c.argoClient.GetResourceTree(ctx, app.Name)
			if err != nil {
				c.logger.Warn("Failed to get resource tree", "app", app.Name, "error", err)
				continue
			}

			// For each resource in the app, create a deployment info object
			for _, node := range tree.Nodes {
				// Skip non-Kubernetes resources or resources with no name/namespace
				if node.Kind == "" || node.Name == "" {
					continue
				}

				// Avoid unnecessary duplicates in the result
				if isResourceAlreadyInResults(result, node.Kind, node.Name, node.Namespace) {
					continue
				}

				// Trace the deployment for this resource
				resourceContext, err := c.TraceResourceDeployment(
					ctx,
					node.Namespace,
					node.Kind,
					node.Name,
				)
				if err != nil {
					c.logger.Warn("Failed to trace resource deployment",
						"kind", node.Kind,
						"name", node.Name,
						"namespace", node.Namespace,
						"error", err)
					continue
				}

				result = append(result, resourceContext)
			}
		}
	}

	c.logger.Info("Found resources affected by commit",
		"projectID", projectID,
		"commitSHA", commitSHA,
		"resourceCount", len(result))

	return result, nil
}

// Helper functions

// extractGitLabProjectPath extracts the GitLab project path from a repo URL
func extractGitLabProjectPath(repoURL string) string {
	// Handle different URL formats

	// Format: https://gitlab.com/namespace/project.git
	if strings.HasPrefix(repoURL, "https://") || strings.HasPrefix(repoURL, "http://") {
		parts := strings.Split(repoURL, "/")
		if len(parts) < 3 {
			return ""
		}

		// Remove ".git" suffix if present
		lastPart := parts[len(parts)-1]
		if strings.HasSuffix(lastPart, ".git") {
			parts[len(parts)-1] = lastPart[:len(lastPart)-4]
		}

		// Reconstruct path without protocol and domain
		domainIndex := 2 // After http:// or https://
		if len(parts) <= domainIndex+1 {
			return ""
		}

		return strings.Join(parts[domainIndex+1:], "/")
	}

	// Format: git@gitlab.com:namespace/project.git
	if strings.HasPrefix(repoURL, "git@") {
		// Split at ":" to get the path part
		parts := strings.Split(repoURL, ":")
		if len(parts) != 2 {
			return ""
		}

		// Remove ".git" suffix if present
		pathPart := strings.TrimSuffix(parts[1], ".git")
		return pathPart
	}

	return ""
}

// extractEnvironmentFromArgoApp tries to determine the environment from an ArgoCD application
func extractEnvironmentFromArgoApp(app models.ArgoApplication) string {
	// Check for environment in labels
	if env, ok := app.Metadata.Labels["environment"]; ok {
		return env
	}
	if env, ok := app.Metadata.Labels["env"]; ok {
		return env
	}

	// Check if environment is in the destination namespace
	if strings.Contains(app.Spec.Destination.Namespace, "prod") {
		return "production"
	}
	if strings.Contains(app.Spec.Destination.Namespace, "staging") {
		return "staging"
	}
	if strings.Contains(app.Spec.Destination.Namespace, "dev") {
		return "development"
	}

	// Check path in source for environment indicators
	if app.Spec.Source.Path != "" {
		if strings.Contains(app.Spec.Source.Path, "prod") {
			return "production"
		}
		if strings.Contains(app.Spec.Source.Path, "staging") {
			return "staging"
		}
		if strings.Contains(app.Spec.Source.Path, "dev") {
			return "development"
		}
	}

	// Default to destination namespace as a fallback
	return app.Spec.Destination.Namespace
}

// isAppSourcedFromProject checks if an ArgoCD application uses a specific GitLab project
func isAppSourcedFromProject(app models.ArgoApplication, projectPath string) bool {
	// Extract project path from app's repo URL
	appProjectPath := extractGitLabProjectPath(app.Spec.Source.RepoURL)

	// Compare paths
	return strings.EqualFold(appProjectPath, projectPath)
}

// isAppAffectedByDiffs checks if application manifests are affected by file changes
func isAppAffectedByDiffs(app models.ArgoApplication, diffs []models.GitLabDiff) bool {
	sourcePath := app.Spec.Source.Path
	if sourcePath == "" {
		// If no specific path is provided, any change could affect the app
		return true
	}

	// Check if any changed file is in the application's source path
	for _, diff := range diffs {
		if strings.HasPrefix(diff.NewPath, sourcePath) || strings.HasPrefix(diff.OldPath, sourcePath) {
			return true
		}
	}

	return false
}

// isResourceAlreadyInResults checks if a resource is already in the results list
func isResourceAlreadyInResults(results []models.ResourceContext, kind, name, namespace string) bool {
	for _, rc := range results {
		if rc.Kind == kind && rc.Name == name && rc.Namespace == namespace {
			return true
		}
	}
	return false
}
