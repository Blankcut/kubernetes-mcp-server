package correlator

import (
	"context"
	"fmt"
	"strings"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TroubleshootCorrelator provides specialized logic for troubleshooting
type TroubleshootCorrelator struct {
	gitOpsCorrelator *GitOpsCorrelator
	k8sClient        *k8s.Client
	logger           *logging.Logger
}

// NewTroubleshootCorrelator creates a new troubleshooting correlator
func NewTroubleshootCorrelator(gitOpsCorrelator *GitOpsCorrelator, k8sClient *k8s.Client, logger *logging.Logger) *TroubleshootCorrelator {
	if logger == nil {
		logger = logging.NewLogger().Named("troubleshoot")
	}

	return &TroubleshootCorrelator{
		gitOpsCorrelator: gitOpsCorrelator,
		k8sClient:        k8sClient,
		logger:           logger,
	}
}

// TroubleshootResource analyzes a resource for common issues
func (tc *TroubleshootCorrelator) TroubleshootResource(ctx context.Context, namespace, kind, name string) (*models.TroubleshootResult, error) {
	tc.logger.Info("Troubleshooting resource", "kind", kind, "name", name, "namespace", namespace)

	// First, trace the resource deployment
	resourceContext, err := tc.gitOpsCorrelator.TraceResourceDeployment(ctx, namespace, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to trace resource deployment: %w", err)
	}

	// Get the raw resource for detailed analysis
	resource, err := tc.k8sClient.GetResource(ctx, kind, namespace, name)
	if err != nil {
		tc.logger.Warn("Failed to get resource for detailed analysis", "error", err)
	}

	// Initialize troubleshooting result
	result := &models.TroubleshootResult{
		ResourceContext: resourceContext,
		Issues:          []models.Issue{},
		Recommendations: []string{},
	}

	// Analyze Kubernetes events for issues
	tc.analyzeKubernetesEvents(resourceContext, result)

	// Analyze resource status and conditions if resource was retrieved
	if resource != nil {
		// Pod-specific analysis
		if strings.EqualFold(kind, "pod") {
			tc.analyzePodStatus(ctx, resource, result)
		}

		// Deployment-specific analysis
		if strings.EqualFold(kind, "deployment") {
			tc.analyzeDeploymentStatus(resource, result)
		}
	}

	// Analyze ArgoCD sync status
	tc.analyzeArgoStatus(resourceContext, result)

	// Analyze GitLab pipeline status
	tc.analyzeGitLabStatus(resourceContext, result)

	// Check if resource is healthy
	if len(result.Issues) == 0 && resource != nil && !tc.isResourceHealthy(resource) {
		issue := models.Issue{
			Source:      "Kubernetes",
			Category:    "UnknownIssue",
			Severity:    "Warning",
			Title:       "Resource Not Healthy",
			Description: fmt.Sprintf("%s %s/%s is not in a healthy state", kind, namespace, name),
		}
		result.Issues = append(result.Issues, issue)
	}

	// Generate recommendations based on issues
	tc.generateRecommendations(result)

	tc.logger.Info("Troubleshooting completed",
		"kind", kind,
		"name", name,
		"namespace", namespace,
		"issueCount", len(result.Issues),
		"recommendationCount", len(result.Recommendations))

	return result, nil
}

// isResourceHealthy checks if a resource is in a healthy state
func (tc *TroubleshootCorrelator) isResourceHealthy(resource *unstructured.Unstructured) bool {
	kind := resource.GetKind()

	// Pod health check
	if strings.EqualFold(kind, "pod") {
		phase, found, _ := unstructured.NestedString(resource.Object, "status", "phase")
		return found && phase == "Running"
	}

	// Deployment health check
	if strings.EqualFold(kind, "deployment") {
		// Check if available replicas match desired replicas
		desiredReplicas, found1, _ := unstructured.NestedInt64(resource.Object, "spec", "replicas")
		availableReplicas, found2, _ := unstructured.NestedInt64(resource.Object, "status", "availableReplicas")
		return found1 && found2 && desiredReplicas == availableReplicas && availableReplicas > 0
	}

	// Default: assume healthy
	return true
}

// analyzeDeploymentStatus analyzes deployment-specific status
func (tc *TroubleshootCorrelator) analyzeDeploymentStatus(deployment *unstructured.Unstructured, result *models.TroubleshootResult) {
	// Check if deployment is ready
	desiredReplicas, found1, _ := unstructured.NestedInt64(deployment.Object, "spec", "replicas")
	availableReplicas, found2, _ := unstructured.NestedInt64(deployment.Object, "status", "availableReplicas")
	readyReplicas, found3, _ := unstructured.NestedInt64(deployment.Object, "status", "readyReplicas")

	if !found1 || !found2 || availableReplicas < desiredReplicas {
		issue := models.Issue{
			Source:      "Kubernetes",
			Category:    "DeploymentNotAvailable",
			Severity:    "Warning",
			Title:       "Deployment Not Fully Available",
			Description: fmt.Sprintf("Deployment has %d/%d available replicas", availableReplicas, desiredReplicas),
		}
		result.Issues = append(result.Issues, issue)
	}

	if !found1 || !found3 || readyReplicas < desiredReplicas {
		issue := models.Issue{
			Source:      "Kubernetes",
			Category:    "DeploymentNotReady",
			Severity:    "Warning",
			Title:       "Deployment Not Fully Ready",
			Description: fmt.Sprintf("Deployment has %d/%d ready replicas", readyReplicas, desiredReplicas),
		}
		result.Issues = append(result.Issues, issue)
	}

	// Check deployment conditions
	conditions, found, _ := unstructured.NestedSlice(deployment.Object, "status", "conditions")
	if found {
		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				continue
			}

			conditionType, _, _ := unstructured.NestedString(condition, "type")
			status, _, _ := unstructured.NestedString(condition, "status")
			reason, _, _ := unstructured.NestedString(condition, "reason")
			message, _, _ := unstructured.NestedString(condition, "message")

			if conditionType == "Available" && status != "True" {
				issue := models.Issue{
					Source:      "Kubernetes",
					Category:    "DeploymentNotAvailable",
					Severity:    "Warning",
					Title:       "Deployment Not Available",
					Description: fmt.Sprintf("Deployment availability issue: %s - %s", reason, message),
				}
				result.Issues = append(result.Issues, issue)
			}

			if conditionType == "Progressing" && status != "True" {
				issue := models.Issue{
					Source:      "Kubernetes",
					Category:    "DeploymentNotProgressing",
					Severity:    "Warning",
					Title:       "Deployment Not Progressing",
					Description: fmt.Sprintf("Deployment progress issue: %s - %s", reason, message),
				}
				result.Issues = append(result.Issues, issue)
			}
		}
	}
}

// analyzePodStatus analyzes pod-specific status information
func (tc *TroubleshootCorrelator) analyzePodStatus(ctx context.Context, pod *unstructured.Unstructured, result *models.TroubleshootResult) {
	// Check pod phase
	phase, found, _ := unstructured.NestedString(pod.Object, "status", "phase")
	if found && phase != "Running" && phase != "Succeeded" {
		issue := models.Issue{
			Source:      "Kubernetes",
			Category:    "PodNotRunning",
			Severity:    "Warning",
			Title:       "Pod Not Running",
			Description: fmt.Sprintf("Pod is in %s state", phase),
		}

		if phase == "Pending" {
			issue.Title = "Pod Pending"
			issue.Description = "Pod is still in Pending state and hasn't started running"
		} else if phase == "Failed" {
			issue.Severity = "Error"
			issue.Title = "Pod Failed"
		}

		result.Issues = append(result.Issues, issue)
	}

	// Check pod conditions
	conditions, found, _ := unstructured.NestedSlice(pod.Object, "status", "conditions")
	if found {
		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				continue
			}

			conditionType, _, _ := unstructured.NestedString(condition, "type")
			status, _, _ := unstructured.NestedString(condition, "status")

			if conditionType == "PodScheduled" && status != "True" {
				issue := models.Issue{
					Source:      "Kubernetes",
					Category:    "SchedulingIssue",
					Severity:    "Warning",
					Title:       "Pod Scheduling Issue",
					Description: "Pod cannot be scheduled onto a node",
				}
				result.Issues = append(result.Issues, issue)
			}

			if conditionType == "Initialized" && status != "True" {
				issue := models.Issue{
					Source:      "Kubernetes",
					Category:    "InitializationIssue",
					Severity:    "Warning",
					Title:       "Pod Initialization Issue",
					Description: "Pod initialization containers have not completed successfully",
				}
				result.Issues = append(result.Issues, issue)
			}

			if conditionType == "ContainersReady" && status != "True" {
				issue := models.Issue{
					Source:      "Kubernetes",
					Category:    "ContainerReadinessIssue",
					Severity:    "Warning",
					Title:       "Container Readiness Issue",
					Description: "One or more containers are not ready",
				}
				result.Issues = append(result.Issues, issue)
			}

			if conditionType == "Ready" && status != "True" {
				issue := models.Issue{
					Source:      "Kubernetes",
					Category:    "PodNotReady",
					Severity:    "Warning",
					Title:       "Pod Not Ready",
					Description: "Pod is not ready to serve traffic",
				}
				result.Issues = append(result.Issues, issue)
			}
		}
	}

	// Check container statuses
	containerStatuses, found, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
	if found {
		tc.analyzeContainerStatuses(containerStatuses, false, result)
	}

	// Check init container statuses if they exist
	initContainerStatuses, found, _ := unstructured.NestedSlice(pod.Object, "status", "initContainerStatuses")
	if found {
		tc.analyzeContainerStatuses(initContainerStatuses, true, result)
	}

	// Check for volume issues
	volumes, found, _ := unstructured.NestedSlice(pod.Object, "spec", "volumes")
	if found {
		// Track PVC usage
		pvcVolumes := []string{}

		for _, v := range volumes {
			volume, ok := v.(map[string]interface{})
			if !ok {
				continue
			}

			// Check for PVC volumes
			pvc, pvcFound, _ := unstructured.NestedMap(volume, "persistentVolumeClaim")
			if pvcFound && pvc != nil {
				claimName, nameFound, _ := unstructured.NestedString(pvc, "claimName")
				if nameFound && claimName != "" {
					pvcVolumes = append(pvcVolumes, claimName)
				}
			}
		}

		// If PVC volumes found, check their status
		if len(pvcVolumes) > 0 {
			for _, pvcName := range pvcVolumes {
				pvc, err := tc.k8sClient.GetResource(ctx, "persistentvolumeclaim", pod.GetNamespace(), pvcName)
				if err != nil {
					issue := models.Issue{
						Source:      "Kubernetes",
						Category:    "VolumeIssue",
						Severity:    "Warning",
						Title:       "PVC Not Found",
						Description: fmt.Sprintf("PersistentVolumeClaim %s not found", pvcName),
					}
					result.Issues = append(result.Issues, issue)
					continue
				}

				phase, phaseFound, _ := unstructured.NestedString(pvc.Object, "status", "phase")
				if !phaseFound || phase != "Bound" {
					issue := models.Issue{
						Source:      "Kubernetes",
						Category:    "VolumeIssue",
						Severity:    "Warning",
						Title:       "PVC Not Bound",
						Description: fmt.Sprintf("PersistentVolumeClaim %s is in %s state", pvcName, phase),
					}
					result.Issues = append(result.Issues, issue)
				}
			}
		}
	}
}

// analyzeContainerStatuses analyzes container status information
func (tc *TroubleshootCorrelator) analyzeContainerStatuses(statuses []interface{}, isInit bool, result *models.TroubleshootResult) {
	containerType := "Container"
	if isInit {
		containerType = "Init Container"
	}

	for _, cs := range statuses {
		containerStatus, ok := cs.(map[string]interface{})
		if !ok {
			continue
		}

		containerName, _, _ := unstructured.NestedString(containerStatus, "name")
		ready, _, _ := unstructured.NestedBool(containerStatus, "ready")
		restartCount, _, _ := unstructured.NestedInt64(containerStatus, "restartCount")

		if !ready {
			// Check for specific container state
			state, stateExists, _ := unstructured.NestedMap(containerStatus, "state")
			if stateExists && state != nil {
				waitingState, waitingExists, _ := unstructured.NestedMap(state, "waiting")
				if waitingExists && waitingState != nil {
					reason, reasonFound, _ := unstructured.NestedString(waitingState, "reason")
					message, messageFound, _ := unstructured.NestedString(waitingState, "message")

					reasonStr := ""
					if reasonFound {
						reasonStr = reason
					}

					messageStr := ""
					if messageFound {
						messageStr = message
					}

					issue := models.Issue{
						Source:      "Kubernetes",
						Category:    "ContainerWaiting",
						Severity:    "Warning",
						Title:       fmt.Sprintf("%s %s Waiting", containerType, containerName),
						Description: fmt.Sprintf("%s is waiting: %s - %s", containerType, reasonStr, messageStr),
					}

					if reason == "CrashLoopBackOff" {
						issue.Category = "CrashLoopBackOff"
						issue.Severity = "Error"
						issue.Title = fmt.Sprintf("%s %s CrashLoopBackOff", containerType, containerName)
					} else if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
						issue.Category = "ImagePullError"
						issue.Title = fmt.Sprintf("%s %s Image Pull Error", containerType, containerName)
					} else if reason == "PodInitializing" || reason == "ContainerCreating" {
						issue.Category = "PodInitializing"
						issue.Title = fmt.Sprintf("%s Still Initializing", containerType)
						issue.Description = fmt.Sprintf("%s is still being created or initialized", containerType)
					}

					result.Issues = append(result.Issues, issue)
				}

				terminatedState, terminatedExists, _ := unstructured.NestedMap(state, "terminated")
				if terminatedExists && terminatedState != nil {
					reason, reasonFound, _ := unstructured.NestedString(terminatedState, "reason")
					exitCode, exitCodeFound, _ := unstructured.NestedInt64(terminatedState, "exitCode")
					message, messageFound, _ := unstructured.NestedString(terminatedState, "message")

					reasonStr := ""
					if reasonFound {
						reasonStr = reason
					}

					messageStr := ""
					if messageFound {
						messageStr = message
					}

					var exitCodeVal int64 = 0
					if exitCodeFound {
						exitCodeVal = exitCode
					}

					if exitCodeVal != 0 {
						issue := models.Issue{
							Source:      "Kubernetes",
							Category:    "ContainerTerminated",
							Severity:    "Error",
							Title:       fmt.Sprintf("%s %s Terminated", containerType, containerName),
							Description: fmt.Sprintf("%s terminated with exit code %d: %s - %s", containerType, exitCodeVal, reasonStr, messageStr),
						}
						result.Issues = append(result.Issues, issue)
					}
				}
			}
		}

		if restartCount > 3 {
			issue := models.Issue{
				Source:      "Kubernetes",
				Category:    "FrequentRestarts",
				Severity:    "Warning",
				Title:       fmt.Sprintf("%s %s Frequent Restarts", containerType, containerName),
				Description: fmt.Sprintf("%s has restarted %d times", containerType, restartCount),
			}
			result.Issues = append(result.Issues, issue)
		}
	}
}

// analyzeKubernetesEvents looks for common issues in Kubernetes events
func (tc *TroubleshootCorrelator) analyzeKubernetesEvents(rc models.ResourceContext, result *models.TroubleshootResult) {
	for _, event := range rc.Events {
		// Look for error events
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
}

// analyzeArgoStatus looks for issues in ArgoCD status
func (tc *TroubleshootCorrelator) analyzeArgoStatus(rc models.ResourceContext, result *models.TroubleshootResult) {
	if rc.ArgoApplication == nil {
		// No ArgoCD application managing this resource
		return
	}

	// Check sync status
	if rc.ArgoSyncStatus != "Synced" {
		issue := models.Issue{
			Source:      "ArgoCD",
			Category:    "SyncIssue",
			Severity:    "Warning",
			Title:       "ArgoCD Sync Issue",
			Description: fmt.Sprintf("Application %s is not synced (status: %s)", rc.ArgoApplication.Name, rc.ArgoSyncStatus),
		}
		result.Issues = append(result.Issues, issue)
	}

	// Check health status
	if rc.ArgoHealthStatus != "Healthy" {
		issue := models.Issue{
			Source:      "ArgoCD",
			Category:    "HealthIssue",
			Severity:    "Warning",
			Title:       "ArgoCD Health Issue",
			Description: fmt.Sprintf("Application %s is not healthy (status: %s)", rc.ArgoApplication.Name, rc.ArgoHealthStatus),
		}
		result.Issues = append(result.Issues, issue)
	}

	// Check for recent sync failures
	for _, history := range rc.ArgoSyncHistory {
		if history.Status == "Failed" {
			issue := models.Issue{
				Source:      "ArgoCD",
				Category:    "SyncFailure",
				Severity:    "Error",
				Title:       "Recent Sync Failure",
				Description: fmt.Sprintf("Sync at %s failed with revision %s", history.DeployedAt.Format("2006-01-02 15:04:05"), history.Revision),
			}
			result.Issues = append(result.Issues, issue)
			break // Only report the most recent failure
		}
	}
}

// analyzeGitLabStatus looks for issues in GitLab pipelines and deployments
func (tc *TroubleshootCorrelator) analyzeGitLabStatus(rc models.ResourceContext, result *models.TroubleshootResult) {
	if rc.GitLabProject == nil {
		// No GitLab project information
		return
	}

	// Check last pipeline status
	if rc.LastPipeline != nil && rc.LastPipeline.Status != "success" {
		severity := "Warning"
		if rc.LastPipeline.Status == "failed" {
			severity = "Error"
		}

		issue := models.Issue{
			Source:      "GitLab",
			Category:    "PipelineIssue",
			Severity:    severity,
			Title:       "GitLab Pipeline Issue",
			Description: fmt.Sprintf("Pipeline #%d status: %s", rc.LastPipeline.ID, rc.LastPipeline.Status),
		}
		result.Issues = append(result.Issues, issue)
	}

	// Check last deployment status
	if rc.LastDeployment != nil && rc.LastDeployment.Status != "success" {
		severity := "Warning"
		if rc.LastDeployment.Status == "failed" {
			severity = "Error"
		}

		issue := models.Issue{
			Source:      "GitLab",
			Category:    "DeploymentIssue",
			Severity:    severity,
			Title:       "GitLab Deployment Issue",
			Description: fmt.Sprintf("Deployment to %s status: %s", rc.LastDeployment.Environment.Name, rc.LastDeployment.Status),
		}
		result.Issues = append(result.Issues, issue)
	}
}

// generateRecommendations creates recommendations based on identified issues
func (tc *TroubleshootCorrelator) generateRecommendations(result *models.TroubleshootResult) {
	// Update the original implementation to include more recommendations
	recommendationMap := make(map[string]bool)

	for _, issue := range result.Issues {
		switch issue.Category {
		case "ImagePullError":
			recommendationMap["Check image name and credentials for accessing private registries."] = true
			recommendationMap["Verify that the image tag exists in the registry."] = true

		case "HealthCheckFailure":
			recommendationMap["Review liveness and readiness probe configuration."] = true
			recommendationMap["Check application logs for errors during startup."] = true

		case "ResourceIssue":
			recommendationMap["Review resource requests and limits in the deployment."] = true
			recommendationMap["Monitor resource usage to determine appropriate values."] = true

		case "CrashLoopBackOff":
			recommendationMap["Check container logs for errors."] = true
			recommendationMap["Verify environment variables and configuration."] = true

		case "SyncIssue", "SyncFailure":
			recommendationMap["Check ArgoCD application manifest for errors."] = true
			recommendationMap["Verify that the target revision exists in the Git repository."] = true

		case "PipelineIssue":
			recommendationMap["Review GitLab pipeline logs for errors."] = true
			recommendationMap["Check if the pipeline configuration is valid."] = true

		case "DeploymentIssue":
			recommendationMap["Check GitLab deployment job logs for errors."] = true
			recommendationMap["Verify deployment environment configuration."] = true

		case "PodNotRunning", "PodNotReady", "PodInitializing":
			recommendationMap["Check pod events for scheduling or initialization issues."] = true
			recommendationMap["Examine init container logs for errors."] = true

		case "InitializationIssue":
			recommendationMap["Check init container logs for errors."] = true
			recommendationMap["Verify that volumes can be mounted properly."] = true

		case "ContainerReadinessIssue":
			recommendationMap["Review readiness probe configuration."] = true
			recommendationMap["Check container logs for application startup issues."] = true

		case "VolumeIssue":
			recommendationMap["Verify that PersistentVolumeClaims are bound."] = true
			recommendationMap["Check if storage classes are properly configured."] = true
			recommendationMap["Ensure sufficient storage space is available on the nodes."] = true

		case "SchedulingIssue":
			recommendationMap["Check if nodes have sufficient resources for the pod."] = true
			recommendationMap["Verify that node selectors or taints are not preventing scheduling."] = true
		}
	}

	// Add generic recommendations if no specific issues found
	if len(result.Issues) == 0 {
		recommendationMap["Check pod logs for errors."] = true
		recommendationMap["Examine Kubernetes events for the resource."] = true
		recommendationMap["Verify network connectivity between components."] = true
	}

	// Convert map to slice
	for rec := range recommendationMap {
		result.Recommendations = append(result.Recommendations, rec)
	}
}
