package mcp

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/claude"
    "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/correlator"
    "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
    "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
    "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
    "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/utils"
    
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ProtocolHandler handles the Model Context Protocol for Kubernetes
type ProtocolHandler struct {
	claudeClient     *claude.Client
	claudeProtocol   *claude.ProtocolHandler
	gitOpsCorrelator *correlator.GitOpsCorrelator
	k8sClient        *k8s.Client
	contextManager   *ContextManager
	promptGenerator  *PromptGenerator
	logger           *logging.Logger
}

// NewProtocolHandler creates a new MCP protocol handler
func NewProtocolHandler(
	claudeClient *claude.Client, 
	gitOpsCorrelator *correlator.GitOpsCorrelator,
	k8sClient *k8s.Client,
	logger *logging.Logger,
) *ProtocolHandler {
	if logger == nil {
		logger = logging.NewLogger().Named("mcp")
	}

	return &ProtocolHandler{
		claudeClient:     claudeClient,
		claudeProtocol:   claude.NewProtocolHandler(claudeClient),
		gitOpsCorrelator: gitOpsCorrelator,
		k8sClient:        k8sClient,
		contextManager:   NewContextManager(100000, logger.Named("context")),
		promptGenerator:  NewPromptGenerator(logger.Named("prompt")),
		logger:           logger,
	}
}

// ProcessRequest processes an MCP request
func (h *ProtocolHandler) ProcessRequest(ctx context.Context, request *models.MCPRequest) (*models.MCPResponse, error) {
    startTime := time.Now()
    h.logger.Info("Processing MCP request", "action", request.Action)

    var resourceContext string
    var err error
    
    // Handle different types of queries
    switch request.Action {
    case "queryResource":
        // If we have pre-populated context, use it
        if request.Context != "" {
            resourceContext = request.Context
        } else {
            // Trace deployment for a specific resource
            resourceInfo, err := h.gitOpsCorrelator.TraceResourceDeployment(
                ctx,
                request.Namespace,
                request.Resource,
                request.Name,
            )
            if err != nil {
                return nil, fmt.Errorf("failed to trace resource deployment: %w", err)
            }

			// For non-namespace resources, enhance with the actual resource data
			if !strings.EqualFold(request.Resource, "namespace") {
				// Get the full resource details
				resource, err := h.k8sClient.GetResource(ctx, request.Resource, request.Namespace, request.Name)
				if err == nil && resource != nil {
					// Add the full resource details to the context
					resourceData, err := utils.ToJSON(resource.Object)
					if err == nil {
						resourceInfo.ResourceData = resourceData
						
						// Extract important deployment-specific information if available
						if strings.EqualFold(request.Resource, "deployment") {
							// Extract replicas info
							specReplicas, found, _ := unstructured.NestedInt64(resource.Object, "spec", "replicas")
							if found {
								if resourceInfo.Metadata == nil {
									resourceInfo.Metadata = make(map[string]interface{})
								}
								resourceInfo.Metadata["desiredReplicas"] = specReplicas
							}
							
							// Extract status replica counts
							statusReplicas, found, _ := unstructured.NestedInt64(resource.Object, "status", "replicas")
							if found {
								if resourceInfo.Metadata == nil {
									resourceInfo.Metadata = make(map[string]interface{})
								}
								resourceInfo.Metadata["currentReplicas"] = statusReplicas
							}
							
							// Extract readyReplicas
							readyReplicas, found, _ := unstructured.NestedInt64(resource.Object, "status", "readyReplicas")
							if found {
								if resourceInfo.Metadata == nil {
									resourceInfo.Metadata = make(map[string]interface{})
								}
								resourceInfo.Metadata["readyReplicas"] = readyReplicas
							}
							
							// Extract availableReplicas
							availableReplicas, found, _ := unstructured.NestedInt64(resource.Object, "status", "availableReplicas")
							if found {
								if resourceInfo.Metadata == nil {
									resourceInfo.Metadata = make(map[string]interface{})
								}
								resourceInfo.Metadata["availableReplicas"] = availableReplicas
							}
							
							// Extract container info
							containers, found, _ := unstructured.NestedSlice(resource.Object, "spec", "template", "spec", "containers")
							if found {
								var containerInfo []map[string]interface{}
								for _, c := range containers {
									container, ok := c.(map[string]interface{})
									if !ok {
										continue
									}
									
									containerData := map[string]interface{}{
										"name": container["name"],
									}
									
									if image, ok := container["image"].(string); ok {
										containerData["image"] = image
									}
									
									if resources, ok := container["resources"].(map[string]interface{}); ok {
										containerData["resources"] = resources
									}
									
									containerInfo = append(containerInfo, containerData)
								}
								
								if resourceInfo.Metadata == nil {
									resourceInfo.Metadata = make(map[string]interface{})
								}
								resourceInfo.Metadata["containers"] = containerInfo
							}
						}
					}
				}
			}
            
            formattedContext, err := h.contextManager.FormatResourceContext(resourceInfo)
            if err != nil {
                return nil, fmt.Errorf("failed to format resource context: %w", err)
            }
            
            resourceContext = formattedContext
        }
        
    case "queryCommit":
        // Find resources affected by a commit
        resources, err := h.gitOpsCorrelator.FindResourcesAffectedByCommit(
            ctx,
            request.ProjectID,
            request.CommitSHA,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to find resources affected by commit: %w", err)
        }
        
        resourceContext, err = h.contextManager.CombineContexts(ctx, resources)
        if err != nil {
            return nil, fmt.Errorf("failed to combine resource contexts: %w", err)
        }
        
    default:
        return nil, fmt.Errorf("unsupported action: %s", request.Action)
    }

    // Generate prompts for Claude
    h.logger.Debug("Generating prompts for Claude")
    systemPrompt := h.promptGenerator.GenerateSystemPrompt()
    userPrompt := h.promptGenerator.GenerateUserPrompt(resourceContext, request.Query)
    
    // Get completion from Claude
    h.logger.Debug("Sending request to Claude", 
        "systemPromptLength", len(systemPrompt),
        "userPromptLength", len(userPrompt))
    
    analysis, err := h.claudeProtocol.GetCompletion(ctx, systemPrompt, userPrompt)
    if err != nil {
        return nil, fmt.Errorf("failed to get completion from Claude: %w", err)
    }

    // Build response
    response := &models.MCPResponse{
        Success:  true,
        Analysis: analysis,
        Message:  fmt.Sprintf("Successfully processed %s request in %v", request.Action, time.Since(startTime)),
    }

    h.logger.Info("MCP request processed successfully", 
        "action", request.Action,
        "duration", time.Since(startTime),
        "responseLength", len(analysis))

    return response, nil
}

// ProcessTroubleshootRequest processes a troubleshooting request with detected issues
func (h *ProtocolHandler) ProcessTroubleshootRequest(ctx context.Context, request *models.MCPRequest, troubleshootResult *models.TroubleshootResult) (*models.MCPResponse, error) {
	startTime := time.Now()
	h.logger.Debug("Processing troubleshoot request")
	
	// Extract issues and recommendations
	var issuesText string
	for i, issue := range troubleshootResult.Issues {
		issuesText += fmt.Sprintf("%d. %s (%s): %s\n", 
			i+1, 
			issue.Title, 
			issue.Severity,
			issue.Description)
	}
	
	var recommendationsText string
	for i, rec := range troubleshootResult.Recommendations {
		recommendationsText += fmt.Sprintf("%d. %s\n", i+1, rec)
	}
	
	// Create a prompt for Claude with the troubleshooting results
	userPrompt := fmt.Sprintf(
		"I'm troubleshooting a Kubernetes %s named '%s' in namespace '%s'.\n\n"+
		"The following issues were detected:\n%s\n"+
		"General recommendations:\n%s\n\n"+
		"Based on these detected issues, please provide specific kubectl commands "+
		"that I can use to troubleshoot and fix the problems. %s",
		request.Resource,
		request.Name,
		request.Namespace,
		issuesText,
		recommendationsText,
		request.Query)
	
	// Generate system prompt
	systemPrompt := h.promptGenerator.GenerateSystemPrompt()
	
	// Get Claude's analysis
	h.logger.Debug("Sending troubleshoot request to Claude", 
		"systemPromptLength", len(systemPrompt),
		"userPromptLength", len(userPrompt))
		
	analysis, err := h.claudeProtocol.GetCompletion(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to get completion for troubleshoot request: %w", err)
	}
	
	// Create response
	response := &models.MCPResponse{
		Success:  true,
		Analysis: analysis,
		Message:  fmt.Sprintf("Successfully processed troubleshoot request in %v", time.Since(startTime)),
	}
	
	h.logger.Info("Troubleshoot request processed successfully", 
		"duration", time.Since(startTime),
		"responseLength", len(analysis))
		
	return response, nil
}

// WithCustomPrompt sets a custom base prompt template
func (h *ProtocolHandler) WithCustomPrompt(template string) *ProtocolHandler {
	h.promptGenerator.WithBasePrompt(template)
	return h
}

// WithMaxContextSize sets the maximum context size
func (h *ProtocolHandler) WithMaxContextSize(size int) *ProtocolHandler {
	h.contextManager = NewContextManager(size, h.logger.Named("context"))
	return h
}