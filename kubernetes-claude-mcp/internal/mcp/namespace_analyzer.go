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
	return h.analyzeNamespace(ctx, namespace, true)
}

// AnalyzeNamespaceFindings is AnalyzeNamespace without the Claude narrative:
// the deterministic findings (warning events, failing scheduled jobs with their
// last log lines) stated plainly. Seconds rather than tens of seconds, for
// callers on a short deadline -- `blankcut diagnose` falls back to it.
func (h *ProtocolHandler) AnalyzeNamespaceFindings(ctx context.Context, namespace string) (*models.NamespaceAnalysisResult, error) {
	return h.analyzeNamespace(ctx, namespace, false)
}

func (h *ProtocolHandler) analyzeNamespace(ctx context.Context, namespace string, narrative bool) (*models.NamespaceAnalysisResult, error) {
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

	// Scheduled work. A failing CronJob leaves every Deployment and Pod healthy,
	// and the Warning event that marks it expires within the hour, so without
	// this the analysis calls a namespace healthy while its jobs fail.
	failingJobs, err := h.k8sClient.FailingCronJobs(ctx, namespace, cronJobLogTailLines)
	if err != nil {
		h.logger.Warn("Failed to check cronjobs", "namespace", namespace, "error", err)
	}
	for i := range failingJobs {
		result.Issues = append(result.Issues, cronJobIssue(&failingJobs[i]))
	}

	if !narrative {
		result.Analysis = deterministicSummary(namespace, result.Issues)
		return result, nil
	}

	// Generate Claude analysis
	analysisPrompt := h.generateNamespaceAnalysisPrompt(namespace, topology, events, failingJobs)
	systemPrompt := h.promptGenerator.GenerateSystemPrompt()

	h.logger.Debug("Sending namespace analysis request to Claude",
		"namespace", namespace,
		"systemPromptLength", len(systemPrompt),
		"analysisPromptLength", len(analysisPrompt))

	analysis, err := h.claudeProtocol.GetCompletion(ctx, systemPrompt, analysisPrompt)
	if err != nil {
		// The deterministic findings stand on their own; losing Claude should
		// cost the narrative, not the whole answer.
		h.logger.Warn("Claude analysis unavailable; returning deterministic findings",
			"namespace", namespace, "error", err)
		result.Analysis = deterministicSummary(namespace, result.Issues)
		return result, nil
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
			switch {
			case strings.HasPrefix(cleanLine, "- "):
				cleanLine = cleanLine[2:]
			case len(cleanLine) > 2 && strings.HasPrefix(cleanLine, "* "):
				cleanLine = cleanLine[2:]
			case len(cleanLine) > 3 &&
				((cleanLine[0] >= '1' && cleanLine[0] <= '9') &&
					(cleanLine[1] == '.' || cleanLine[1] == ')') &&
					(cleanLine[2] == ' ')):
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

// cronJobLogTailLines is how much of a failed run's log reaches the analysis.
const cronJobLogTailLines = 40

// cronJobIssue turns a failing CronJob into an issue a person can act on.
func cronJobIssue(cj *k8s.CronJobHealth) models.Issue {
	since := "it has never succeeded"
	if !cj.LastSuccess.IsZero() {
		since = "last success " + cj.LastSuccess.UTC().Format(time.RFC3339)
	}
	desc := fmt.Sprintf("Scheduled job %s (%s) did not succeed on its last run at %s; %s.",
		cj.Name, cj.Schedule, cj.LastSchedule.UTC().Format(time.RFC3339), since)
	switch {
	case cj.LogTail != "":
		desc += " End of the failed run's log:\n" + lastLines(cj.LogTail, 8)
	case cj.LogNote != "":
		desc += " " + cj.LogNote + "."
	}
	return models.Issue{
		Source:      "Kubernetes",
		Category:    "CronJobFailing",
		Severity:    "Warning",
		Title:       "Scheduled job failing: " + cj.Name,
		Description: desc,
	}
}

// lastLines returns at most n trailing non-empty lines.
func lastLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// deterministicSummary is the analysis when Claude is unavailable: the issues,
// stated plainly.
func deterministicSummary(namespace string, issues []models.Issue) string {
	if len(issues) == 0 {
		return fmt.Sprintf("No issues found in %s.", namespace)
	}
	out := fmt.Sprintf("%d issue(s) found in %s:\n", len(issues), namespace)
	for _, is := range issues {
		out += fmt.Sprintf("\n- %s: %s", is.Title, is.Description)
	}
	return out
}

// generateNamespaceAnalysisPrompt creates a prompt for namespace analysis
func (h *ProtocolHandler) generateNamespaceAnalysisPrompt(namespace string, topology *k8s.NamespaceTopology, events []models.K8sEvent, failingJobs []k8s.CronJobHealth) string {
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
			if relType != "" {
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

	// Failing scheduled jobs, with the log of the last failed run: the cause is
	// never in the CronJob object, and usually is in that log.
	if len(failingJobs) > 0 {
		prompt += "## Failing Scheduled Jobs\n\n"
		for _, cj := range failingJobs {
			last := "never"
			if !cj.LastSuccess.IsZero() {
				last = cj.LastSuccess.UTC().Format(time.RFC3339)
			}
			prompt += fmt.Sprintf("### CronJob %s (schedule %q)\n- last scheduled run: %s (did not succeed)\n- last success: %s\n",
				cj.Name, cj.Schedule, cj.LastSchedule.UTC().Format(time.RFC3339), last)
			if cj.LastFailedJob != "" {
				prompt += fmt.Sprintf("- last failed run: %s\n", cj.LastFailedJob)
			}
			if cj.LogTail != "" {
				prompt += "\nEnd of that run's log:\n```\n" + lastLines(cj.LogTail, cronJobLogTailLines) + "\n```\n"
			} else if cj.LogNote != "" {
				prompt += "- log: " + cj.LogNote + "\n"
			}
			prompt += "\n"
		}
		prompt += "Explain what the log says went wrong for each failing job. If the log shows an error " +
			"that was never serialized (for example `[object Object]`), say so plainly: the fix then starts " +
			"with the application logging the real error.\n\n"
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
