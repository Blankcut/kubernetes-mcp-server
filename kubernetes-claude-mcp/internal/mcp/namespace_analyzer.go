package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	k8s "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// NamespaceAnalysisResult contains the analysis of a namespace's resources
type NamespaceAnalysisResult struct {
	Namespace             string                     `json:"namespace"`
	ResourceCounts        map[string]int             `json:"resourceCounts"`
	HealthStatus          map[string]map[string]int  `json:"healthStatus"`
	ResourceRelationships []k8s.ResourceRelationship `json:"resourceRelationships"`
	Issues                []models.Issue             `json:"issues"`
	Recommendations       []string                   `json:"recommendations"`
	Analysis              string                     `json:"analysis"`
}

// AnalyzeNamespace analyzes all resources in a namespace using Claude
func (h *ProtocolHandler) AnalyzeNamespace(ctx context.Context, namespace string) (*models.NamespaceAnalysisResult, error) {
	startTime := time.Now()
	h.logger.Info("Analyzing namespace", "namespace", namespace)

	// Get namespace topology
	topology, err := h.k8sClient.GetNamespaceTopology(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace topology: %w", err)
	}

	// Initialize result
	result := &models.NamespaceAnalysisResult{
		Namespace:       namespace,
		ResourceCounts:  make(map[string]int),
		HealthStatus:    make(map[string]map[string]int),
		Issues:          []models.Issue{},
		Recommendations: []string{},
	}

	// Extract resource counts
	for kind, resources := range topology.Resources {
		result.ResourceCounts[kind] = len(resources)
	}

	// Extract health status
	for kind, statusMap := range topology.Health {
		healthCounts := make(map[string]int)
		for _, status := range statusMap {
			healthCounts[status]++
		}
		result.HealthStatus[kind] = healthCounts
	}

	// Add relationships - Convert from k8s.ResourceRelationship to models.ResourceRelationship
	for _, rel := range topology.Relationships {
		modelRel := models.ResourceRelationship{
			SourceKind:      rel.SourceKind,
			SourceName:      rel.SourceName,
			SourceNamespace: rel.SourceNamespace,
			TargetKind:      rel.TargetKind,
			TargetName:      rel.TargetName,
			TargetNamespace: rel.TargetNamespace,
			RelationType:    rel.RelationType,
		}
		result.ResourceRelationships = append(result.ResourceRelationships, modelRel)
	}

	// Get events for the namespace
	events, err := h.k8sClient.GetNamespaceEvents(ctx, namespace)
	if err != nil {
		h.logger.Warn("Failed to get namespace events", "error", err)
	}

	// Identify issues from events
	for _, event := range events {
		if event.Type == "Warning" {
			issue := models.Issue{
				Source:      "Kubernetes",
				Severity:    "Warning",
				Description: fmt.Sprintf("%s: %s", event.Reason, event.Message),
			}

			// Categorize common issues
			switch {
			case strings.Contains(event.Reason, "Failed") && strings.Contains(event.Message, "ImagePull"):
				issue.Category = "ImagePullError"
				issue.Title = "Image Pull Failure"

			case strings.Contains(event.Reason, "Unhealthy"):
				issue.Category = "HealthCheckFailure"
				issue.Title = "Health Check Failure"

			case strings.Contains(event.Message, "memory"):
				issue.Category = "ResourceIssue"
				issue.Title = "Memory Resource Issue"

			case strings.Contains(event.Message, "cpu"):
				issue.Category = "ResourceIssue"
				issue.Title = "CPU Resource Issue"

			case strings.Contains(event.Reason, "BackOff"):
				issue.Category = "CrashLoopBackOff"
				issue.Title = "Container Crash Loop"

			default:
				issue.Category = "OtherWarning"
				issue.Title = "Kubernetes Warning"
			}

			result.Issues = append(result.Issues, issue)
		}
	}

	// Generate Claude analysis
	analysisPrompt := h.generateNamespaceAnalysisPrompt(namespace, topology, events)
	systemPrompt := h.promptGenerator.GenerateSystemPrompt()

	h.logger.Debug("Sending namespace analysis request to Claude",
		"namespace", namespace,
		"systemPromptLength", len(systemPrompt),
		"analysisPromptLength", len(analysisPrompt))

	analysis, err := h.claudeProtocol.GetCompletion(ctx, systemPrompt, analysisPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to get completion for namespace analysis: %w", err)
	}

	// Extract recommendations from analysis
	lines := strings.Split(analysis, "\n")
	inRecommendations := false

	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "recommendation") ||
			strings.Contains(strings.ToLower(line), "recommendations") ||
			strings.Contains(strings.ToLower(line), "suggest") {
			inRecommendations = true
			continue
		}

		if inRecommendations && strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
			// Remove leading dash or number if it exists
			cleanLine := strings.TrimSpace(line)
			if strings.HasPrefix(cleanLine, "- ") {
				cleanLine = cleanLine[2:]
			} else if len(cleanLine) > 2 && strings.HasPrefix(cleanLine, "* ") {
				cleanLine = cleanLine[2:]
			} else if len(cleanLine) > 3 &&
				((cleanLine[0] >= '1' && cleanLine[0] <= '9') &&
					(cleanLine[1] == '.' || cleanLine[1] == ')') &&
					(cleanLine[2] == ' ')) {
				cleanLine = cleanLine[3:]
			}

			if cleanLine != "" && len(result.Recommendations) < 10 {
				result.Recommendations = append(result.Recommendations, cleanLine)
			}
		}
	}

	result.Analysis = analysis

	h.logger.Info("Namespace analysis completed",
		"namespace", namespace,
		"duration", time.Since(startTime),
		"issueCount", len(result.Issues),
		"recommendationCount", len(result.Recommendations))

	return result, nil
}

// generateNamespaceAnalysisPrompt creates a prompt for namespace analysis
func (h *ProtocolHandler) generateNamespaceAnalysisPrompt(namespace string, topology *k8s.NamespaceTopology, events []models.K8sEvent) string {
	// Start with namespace overview
	prompt := fmt.Sprintf("# Namespace Analysis: %s\n\n", namespace)

	// Add resource summary
	prompt += "## Resource Summary\n\n"
	for kind, resources := range topology.Resources {
		prompt += fmt.Sprintf("- %s: %d resources\n", kind, len(resources))
	}
	prompt += "\n"

	// Add health status summary
	prompt += "## Health Status\n\n"
	for kind, statusMap := range topology.Health {
		prompt += fmt.Sprintf("### %s Health\n", kind)

		// Count the statuses
		healthCounts := make(map[string]int)
		for _, status := range statusMap {
			healthCounts[status]++
		}

		// List the counts
		for status, count := range healthCounts {
			prompt += fmt.Sprintf("- %s: %d resources\n", status, count)
		}

		// List unhealthy resources
		unhealthyResources := []string{}
		for name, status := range statusMap {
			if status == "unhealthy" {
				unhealthyResources = append(unhealthyResources, name)
			}
		}

		if len(unhealthyResources) > 0 {
			prompt += "\nUnhealthy resources:\n"
			for _, name := range unhealthyResources {
				prompt += fmt.Sprintf("- %s\n", name)
			}
		}

		prompt += "\n"
	}

	// Add relationship summary
	if len(topology.Relationships) > 0 {
		prompt += "## Resource Relationships\n\n"

		// Group by relationship type
		relationshipsByType := make(map[string][]string)
		for _, rel := range topology.Relationships {
			key := rel.RelationType
			relationshipsByType[key] = append(
				relationshipsByType[key],
				fmt.Sprintf("%s/%s -> %s/%s",
					rel.SourceKind, rel.SourceName,
					rel.TargetKind, rel.TargetName))
		}

		// List relationships by type
		for relType, relations := range relationshipsByType {
			// Capitalize first letter of relationship type
			capitalizedType := relType
			if len(relType) > 0 {
				capitalizedType = strings.ToUpper(relType[:1]) + relType[1:]
			}
			prompt += fmt.Sprintf("### %s Relationships\n", capitalizedType)
			for _, rel := range relations {
				prompt += fmt.Sprintf("- %s\n", rel)
			}
			prompt += "\n"
		}
	}

	// Add recent events
	if len(events) > 0 {
		prompt += "## Recent Events\n\n"

		// Group events by type
		warningEvents := []models.K8sEvent{}
		normalEvents := []models.K8sEvent{}

		for _, event := range events {
			if event.Type == "Warning" {
				warningEvents = append(warningEvents, event)
			} else {
				normalEvents = append(normalEvents, event)
			}
		}

		// Add warning events first (limited to 10)
		if len(warningEvents) > 0 {
			prompt += "### Warning Events\n"
			count := 0
			for _, event := range warningEvents {
				if count >= 10 {
					break
				}
				prompt += fmt.Sprintf("- [%s] %s: %s (%s)\n",
					event.LastTime.Format(time.RFC3339),
					event.Reason,
					event.Message,
					fmt.Sprintf("%s/%s", event.Object.Kind, event.Object.Name))
				count++
			}
			prompt += "\n"
		}

		// Add a few normal events (limited to 5)
		if len(normalEvents) > 0 {
			prompt += "### Normal Events\n"
			count := 0
			for _, event := range normalEvents {
				if count >= 5 {
					break
				}
				prompt += fmt.Sprintf("- [%s] %s: %s (%s)\n",
					event.LastTime.Format(time.RFC3339),
					event.Reason,
					event.Message,
					fmt.Sprintf("%s/%s", event.Object.Kind, event.Object.Name))
				count++
			}
			prompt += "\n"
		}
	}

	// Add analysis request
	prompt += "## Analysis Request\n\n"
	prompt += "Based on the information above, please provide a comprehensive analysis of this Kubernetes namespace, including:\n\n"
	prompt += "1. Overall health assessment\n"
	prompt += "2. Identification of any issues or problems\n"
	prompt += "3. Analysis of resource relationships and dependencies\n"
	prompt += "4. Potential bottlenecks or misconfigurations\n"
	prompt += "5. Security concerns (if any can be identified)\n"
	prompt += "6. Specific recommendations for improvement\n\n"
	prompt += "Please format your analysis with clear sections and provide specific, actionable recommendations that would help improve the reliability, efficiency, and security of this namespace."

	return prompt
}
