package mcp

import (
	"strings"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// PromptGenerator generates prompts for Claude based on context and user queries
type PromptGenerator struct {
	basePromptTemplate string
	logger             *logging.Logger
}

// NewPromptGenerator creates a new prompt generator
func NewPromptGenerator(logger *logging.Logger) *PromptGenerator {
	if logger == nil {
		logger = logging.NewLogger().Named("prompt")
	}
	
	return &PromptGenerator{
		basePromptTemplate: defaultBasePrompt,
		logger:             logger,
	}
}

// WithBasePrompt sets a custom base prompt template
func (pg *PromptGenerator) WithBasePrompt(template string) *PromptGenerator {
	pg.basePromptTemplate = template
	pg.logger.Debug("Set custom base prompt template", "length", len(template))
	return pg
}

// GenerateSystemPrompt creates a system prompt for Claude based on the request
func (pg *PromptGenerator) GenerateSystemPrompt() string {
	pg.logger.Debug("Generating system prompt")
	return pg.basePromptTemplate
}

// GenerateUserPrompt creates a user prompt containing the context and query
func (pg *PromptGenerator) GenerateUserPrompt(context, query string) string {
	// Clean up the query
	cleanQuery := strings.TrimSpace(query)
	
	pg.logger.Debug("Generating user prompt", 
		"contextLength", len(context), 
		"queryLength", len(cleanQuery))
	
	// Build the prompt
	prompt := "Here is the GitOps context for the Kubernetes resources you requested:\n\n"
	prompt += context
	prompt += "\n\nBased on this context, please answer the following question or perform the requested analysis:\n\n"
	prompt += cleanQuery
	
	return prompt
}

// Default base prompt template
const defaultBasePrompt = `You are a Kubernetes GitOps assistant that helps users troubleshoot and understand their Kubernetes clusters.
You have access to information from Kubernetes, ArgoCD, and GitLab through the provided context.
When answering questions:
1. Only reference resources and information that are explicitly shown in the context.
2. If asked about resources or information not in the context, explain that you don't have that information.
3. Provide accurate technical details when analyzing Kubernetes configurations and GitOps workflows.
4. Look for connections between issues in different systems (e.g., a failed GitLab pipeline causing ArgoCD sync issues).
5. Suggest best practices and potential fixes when appropriate.
6. Format YAML, JSON, and code cleanly when showing configurations or examples.
7. When suggesting kubectl, argocd, or GitLab CLI commands, ensure they're correctly formatted.

You can help with tasks like:
- Explaining the current state of resources and their deployment history
- Analyzing configurations for potential issues
- Suggesting improvements to deployments
- Explaining the relationship between Git commits, CI/CD pipelines, and deployed resources
- Troubleshooting failed deployments by correlating information across systems
- Identifying root causes of issues in the GitOps pipeline`