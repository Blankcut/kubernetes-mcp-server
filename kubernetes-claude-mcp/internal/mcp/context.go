package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/utils"
)

// ContextManager handles the creation and management of context for Claude
type ContextManager struct {
	maxContextSize int
	logger         *logging.Logger
}

// NewContextManager creates a new context manager
func NewContextManager(maxContextSize int, logger *logging.Logger) *ContextManager {
	if maxContextSize <= 0 {
		maxContextSize = 100000
	}

	if logger == nil {
		logger = logging.NewLogger().Named("context")
	}

	return &ContextManager{
		maxContextSize: maxContextSize,
		logger:         logger,
	}
}

// FormatResourceContext formats a resource context for Claude
func (cm *ContextManager) FormatResourceContext(rc models.ResourceContext) (string, error) {
	cm.logger.Debug("Formatting resource context", 
		"kind", rc.Kind, 
		"name", rc.Name, 
		"namespace", rc.Namespace)
	
	var formattedContext string

	// Format the basic resource information
	formattedContext += fmt.Sprintf("# Kubernetes Resource: %s/%s\n", rc.Kind, rc.Name)
	if rc.Namespace != "" {
		formattedContext += fmt.Sprintf("Namespace: %s\n", rc.Namespace)
	}
	formattedContext += fmt.Sprintf("API Version: %s\n\n", rc.APIVersion)

	// Format ArgoCD information if available
	if rc.ArgoApplication != nil {
		formattedContext += "## ArgoCD Application\n"
		formattedContext += fmt.Sprintf("Name: %s\n", rc.ArgoApplication.Name)
		formattedContext += fmt.Sprintf("Sync Status: %s\n", rc.ArgoSyncStatus)
		formattedContext += fmt.Sprintf("Health Status: %s\n", rc.ArgoHealthStatus)
		
		if rc.ArgoApplication.Spec.Source.RepoURL != "" {
			formattedContext += fmt.Sprintf("Source: %s\n", rc.ArgoApplication.Spec.Source.RepoURL)
			formattedContext += fmt.Sprintf("Path: %s\n", rc.ArgoApplication.Spec.Source.Path)
			formattedContext += fmt.Sprintf("Target Revision: %s\n", rc.ArgoApplication.Spec.Source.TargetRevision)
		}
		
		formattedContext += "\n"
		
		// Add recent sync history
		if len(rc.ArgoSyncHistory) > 0 {
			formattedContext += "### Recent Sync History\n"
			for i, history := range rc.ArgoSyncHistory {
				formattedContext += fmt.Sprintf("%d. [%s] Revision: %s, Status: %s\n", 
					i+1, 
					history.DeployedAt.Format(time.RFC3339), 
					history.Revision, 
					history.Status)
			}
			formattedContext += "\n"
		}
	}

	// Format GitLab information if available
	if rc.GitLabProject != nil {
		formattedContext += "## GitLab Project\n"
		formattedContext += fmt.Sprintf("Name: %s\n", rc.GitLabProject.PathWithNamespace)
		formattedContext += fmt.Sprintf("URL: %s\n\n", rc.GitLabProject.WebURL)
		
		// Add last pipeline information
		if rc.LastPipeline != nil {
			formattedContext += "### Last Pipeline\n"
			
			// Handle pipeline CreatedAt timestamp
			var pipelineTimestamp string
			switch createdAt := rc.LastPipeline.CreatedAt.(type) {
			case int64:
				pipelineTimestamp = time.Unix(createdAt, 0).Format(time.RFC3339)
			case float64:
				pipelineTimestamp = time.Unix(int64(createdAt), 0).Format(time.RFC3339)
			case string:
				// Try to parse the string timestamp
				parsed, err := time.Parse(time.RFC3339, createdAt)
				if err != nil {
					// Try alternative format
					parsed, err = time.Parse("2006-01-02T15:04:05.000Z", createdAt)
					if err != nil {
						// Use raw string if parsing fails
						pipelineTimestamp = createdAt
					} else {
						pipelineTimestamp = parsed.Format(time.RFC3339)
					}
				} else {
					pipelineTimestamp = parsed.Format(time.RFC3339)
				}
			default:
				pipelineTimestamp = "unknown timestamp"
			}
			
			formattedContext += fmt.Sprintf("Status: %s\n", rc.LastPipeline.Status)
			formattedContext += fmt.Sprintf("Ref: %s\n", rc.LastPipeline.Ref)
			formattedContext += fmt.Sprintf("SHA: %s\n", rc.LastPipeline.SHA)
			formattedContext += fmt.Sprintf("Created At: %s\n\n", pipelineTimestamp)
		}
		
		// Add last deployment information
		if rc.LastDeployment != nil {
			formattedContext += "### Last Deployment\n"
			
			// Handle deployment CreatedAt timestamp
			var deploymentTimestamp string
			switch createdAt := rc.LastDeployment.CreatedAt.(type) {
			case int64:
				deploymentTimestamp = time.Unix(createdAt, 0).Format(time.RFC3339)
			case float64:
				deploymentTimestamp = time.Unix(int64(createdAt), 0).Format(time.RFC3339)
			case string:
				// Try to parse the string timestamp
				parsed, err := time.Parse(time.RFC3339, createdAt)
				if err != nil {
					// Try alternative format
					parsed, err = time.Parse("2006-01-02T15:04:05.000Z", createdAt)
					if err != nil {
						// Use raw string if parsing fails
						deploymentTimestamp = createdAt
					} else {
						deploymentTimestamp = parsed.Format(time.RFC3339)
					}
				} else {
					deploymentTimestamp = parsed.Format(time.RFC3339)
				}
			default:
				deploymentTimestamp = "unknown timestamp"
			}
			
			formattedContext += fmt.Sprintf("Status: %s\n", rc.LastDeployment.Status)
			formattedContext += fmt.Sprintf("Environment: %s\n", rc.LastDeployment.Environment.Name)
			formattedContext += fmt.Sprintf("Created At: %s\n\n", deploymentTimestamp)
		}
		
		// Add recent commits
		if len(rc.RecentCommits) > 0 {
			formattedContext += "### Recent Commits\n"
			for i, commit := range rc.RecentCommits {
				// Handle commit CreatedAt timestamp
				var commitTimestamp string
				switch createdAt := commit.CreatedAt.(type) {
				case int64:
					commitTimestamp = time.Unix(createdAt, 0).Format(time.RFC3339)
				case float64:
					commitTimestamp = time.Unix(int64(createdAt), 0).Format(time.RFC3339)
				case string:
					// Try to parse the string timestamp
					parsed, err := time.Parse(time.RFC3339, createdAt)
					if err != nil {
						// Try alternative format
						parsed, err = time.Parse("2006-01-02T15:04:05.000Z", createdAt)
						if err != nil {
							// Use raw string if parsing fails
							commitTimestamp = createdAt
						} else {
							commitTimestamp = parsed.Format(time.RFC3339)
						}
					} else {
						commitTimestamp = parsed.Format(time.RFC3339)
					}
				default:
					commitTimestamp = "unknown timestamp"
				}
				
				formattedContext += fmt.Sprintf("%d. [%s] %s by %s: %s\n", 
					i+1, 
					commitTimestamp, 
					commit.ShortID, 
					commit.AuthorName, 
					commit.Title)
			}
			formattedContext += "\n"
		}
	}

	// Format Kubernetes events
	if len(rc.Events) > 0 {
		formattedContext += "## Recent Kubernetes Events\n"
		for i, event := range rc.Events {
			formattedContext += fmt.Sprintf("%d. [%s] %s: %s\n", 
				i+1, 
				event.Type, 
				event.Reason, 
				event.Message)
		}
		formattedContext += "\n"
	}

	// Add related resources
	if len(rc.RelatedResources) > 0 {
		formattedContext += "## Related Resources\n"
		for _, resource := range rc.RelatedResources {
			formattedContext += fmt.Sprintf("- %s\n", resource)
		}
		formattedContext += "\n"
	}

	// Add errors if any
	if len(rc.Errors) > 0 {
		formattedContext += "## Errors in Data Collection\n"
		for _, err := range rc.Errors {
			formattedContext += fmt.Sprintf("- %s\n", err)
		}
		formattedContext += "\n"
	}

	// Ensure context doesn't exceed max size
	if len(formattedContext) > cm.maxContextSize {
		cm.logger.Debug("Context exceeds maximum size, truncating", 
			"originalSize", len(formattedContext), 
			"maxSize", cm.maxContextSize)
		formattedContext = utils.TruncateContextSmartly(formattedContext, cm.maxContextSize)
	}

	cm.logger.Debug("Formatted resource context", 
		"kind", rc.Kind, 
		"name", rc.Name, 
		"contextSize", len(formattedContext))
	return formattedContext, nil
}

// CombineContexts combines multiple resource contexts into a single context
func (cm *ContextManager) CombineContexts(ctx context.Context, resourceContexts []models.ResourceContext) (string, error) {
	cm.logger.Debug("Combining resource contexts", "count", len(resourceContexts))
	
	var combinedContext string
	
	combinedContext += fmt.Sprintf("# Kubernetes GitOps Context (%d resources)\n\n", len(resourceContexts))
	
	// Add context for each resource
	for i, rc := range resourceContexts {
		resourceContext, err := cm.FormatResourceContext(rc)
		if err != nil {
			return "", fmt.Errorf("failed to format resource context #%d: %w", i+1, err)
		}
		
		combinedContext += fmt.Sprintf("--- RESOURCE %d/%d ---\n", i+1, len(resourceContexts))
		combinedContext += resourceContext
		combinedContext += "------------------------\n\n"
	}
	
	// Ensure combined context doesn't exceed max size
	if len(combinedContext) > cm.maxContextSize {
		cm.logger.Debug("Combined context exceeds maximum size, truncating", 
			"originalSize", len(combinedContext), 
			"maxSize", cm.maxContextSize)
		combinedContext = utils.TruncateContextSmartly(combinedContext, cm.maxContextSize)
	}
	
	cm.logger.Debug("Combined resource contexts", 
		"resourceCount", len(resourceContexts), 
		"contextSize", len(combinedContext))
	return combinedContext, nil
}