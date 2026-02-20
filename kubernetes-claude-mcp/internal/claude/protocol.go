package claude

import (
	"context"
	"fmt"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/utils"
)

// ProtocolHandler manages protocol-specific Claude operations
type ProtocolHandler struct {
	client *Client
}

// NewProtocolHandler creates a new Claude protocol handler
func NewProtocolHandler(client *Client) *ProtocolHandler {
	return &ProtocolHandler{
		client: client,
	}
}

// GetCompletion gets a completion from Claude with context management
func (h *ProtocolHandler) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Check if combined prompts are too large and truncate if needed
	const maxPromptSize = 100000

	if len(systemPrompt)+len(userPrompt) > maxPromptSize {
		// Prioritize the user prompt over system prompt for truncation
		maxUserPromptSize := maxPromptSize - len(systemPrompt) - 100 // Buffer

		if maxUserPromptSize < 1000 {
			// System prompt is too large, truncate it
			systemPrompt = utils.TruncateContent(systemPrompt, maxPromptSize/2)
			maxUserPromptSize = maxPromptSize/2 - 100 // Adding buffer
		}

		userPrompt = utils.TruncateContextSmartly(userPrompt, maxUserPromptSize)
	}

	// Create messages
	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}

	// Get completion
	response, err := h.client.Complete(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("claude completion failed: %w", err)
	}

	return response, nil
}

// TruncateContent ensures the content fits within Claude's context window
// This is a helper specifically for the Claude protocol
func (h *ProtocolHandler) TruncateContent(content string, maxSize int) string {
	return utils.TruncateContent(content, maxSize)
}
