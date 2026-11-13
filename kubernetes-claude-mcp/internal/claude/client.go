package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/telemetry"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// Client handles communication with the Claude API
type Client struct {
	apiKey      string
	baseURL     string
	modelID     string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
	logger      *logging.Logger
	// federation is non-nil only when Workload Identity Federation is
	// configured. When nil, requests authenticate with apiKey exactly as before.
	federation *federationTokenSource
	// reporter is nil unless model call reporting is configured. It is told
	// about each call after the call has finished, and cannot affect it.
	reporter CallReporter
}

// CallReporter is told about each Messages API call once it has finished:
// which model, how many tokens, how long. *telemetry.Reporter is one.
type CallReporter interface {
	ReportModelCall(call *telemetry.ModelCall)
}

// SetCallReporter makes the client report each model call to r. Call it
// before the client is used; passing nil turns reporting off.
func (c *Client) SetCallReporter(r CallReporter) {
	c.reporter = r
}

// Message represents a message in the Claude conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest represents a request to the Claude API
type CompletionRequest struct {
	Model       string    `json:"model"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

// ContentItem represents an item in the content array of a response
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CompletionResponse represents a response from the Claude API
type CompletionResponse struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`
	Model   string        `json:"model"`
	Content []ContentItem `json:"content"`
	Usage   Usage         `json:"usage"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// Tokens read from and written to the prompt cache. InputTokens does not
	// include them.
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// NewClient creates a new Claude API client
func NewClient(cfg *ClaudeConfig, logger *logging.Logger) *Client {
	if logger == nil {
		logger = logging.NewLogger().Named("claude")
	}

	httpClient := &http.Client{
		Timeout: 120 * time.Second,
	}

	client := &Client{
		apiKey:      cfg.APIKey,
		baseURL:     cfg.BaseURL,
		modelID:     cfg.ModelID,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		httpClient:  httpClient,
		logger:      logger,
	}

	if cfg.Federation.Enabled() {
		client.federation = newFederationTokenSource(&cfg.Federation, cfg.BaseURL, httpClient)
		logger.Info("Using Workload Identity Federation for Claude authentication",
			"federationRuleID", cfg.Federation.FederationRuleID)
	}

	return client
}

// ClaudeConfig holds configuration for the Claude API client
//
//nolint:revive // ClaudeConfig is intentionally named to be explicit about the service
type ClaudeConfig struct {
	APIKey      string  `yaml:"apiKey"`
	BaseURL     string  `yaml:"baseURL"`
	ModelID     string  `yaml:"modelID"`
	MaxTokens   int     `yaml:"maxTokens"`
	Temperature float64 `yaml:"temperature"`
	// Federation is optional. Leave it unset to authenticate with APIKey.
	Federation FederationConfig `yaml:"federation"`
}

// Complete sends a completion request to the Claude API
func (c *Client) Complete(ctx context.Context, messages []Message) (string, error) {
	c.logger.Debug("Sending completion request",
		"model", c.modelID,
		"messageCount", len(messages))

	// Extract system message if present
	var systemPrompt string
	var userMessages []Message

	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
		} else {
			userMessages = append(userMessages, msg)
		}
	}

	reqBody := CompletionRequest{
		Model:       c.modelID,
		System:      systemPrompt,
		Messages:    userMessages,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/messages",
		bytes.NewBuffer(reqJSON),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	// Federated tokens go on Authorization as a bearer credential; a static API
	// key goes on x-api-key. Never both -- sending an API key alongside a
	// federated token defeats the point of having no long-lived secret.
	if c.federation != nil {
		token, tokenErr := c.federation.Token(ctx)
		if tokenErr != nil {
			return "", fmt.Errorf("failed to obtain federated access token: %w", tokenErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("x-api-key", c.apiKey)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.reportCall(ctx, start, nil, transportErrorType(err))
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.reportCall(ctx, start, nil, transportErrorType(err))
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.reportCall(ctx, start, nil, apiErrorType(resp.StatusCode, body))
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, body)
	}

	var completionResponse CompletionResponse
	if err := json.Unmarshal(body, &completionResponse); err != nil {
		c.reportCall(ctx, start, nil, "invalid_response")
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}
	c.reportCall(ctx, start, &completionResponse, "")

	// Extract text from content array
	var responseText string
	for _, content := range completionResponse.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	c.logger.Debug("Received completion response",
		"model", completionResponse.Model,
		"inputTokens", completionResponse.Usage.InputTokens,
		"outputTokens", completionResponse.Usage.OutputTokens)

	return responseText, nil
}

// reportCall tells the reporter about one finished call. It sends counts,
// names and times: never the prompt, never the response text. Nothing it does
// can reach the caller, including a panic in a reporter.
func (c *Client) reportCall(ctx context.Context, start time.Time, resp *CompletionResponse, errorType string) {
	if c.reporter == nil {
		return
	}
	defer func() { _ = recover() }()

	call := &telemetry.ModelCall{
		Start:        start,
		End:          time.Now(),
		Provider:     "anthropic",
		RequestModel: c.modelID,
		ErrorType:    errorType,
		Analysis:     analysisFrom(ctx),
	}
	if resp != nil {
		call.ResponseModel = resp.Model
		call.ResponseID = resp.ID
		call.Usage = &telemetry.Usage{
			InputTokens:      int64(resp.Usage.InputTokens),
			OutputTokens:     int64(resp.Usage.OutputTokens),
			CacheReadTokens:  int64(resp.Usage.CacheReadInputTokens),
			CacheWriteTokens: int64(resp.Usage.CacheCreationInputTokens),
		}
	}
	c.reporter.ReportModelCall(call)
}

// transportErrorType names why a request got no usable response.
func transportErrorType(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	return "connection_error"
}

// apiErrorType is the API's own error type ("rate_limit_error",
// "overloaded_error") when the body carries one, else the HTTP status code.
// Only the type is read: an error's message can quote the request.
func apiErrorType(status int, body []byte) string {
	var parsed struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Type != "" {
		return parsed.Error.Type
	}
	return strconv.Itoa(status)
}

type analysisKey struct{}

// The analyses the server asks the model for. A model call is reported with
// the one it was made for, so usage can be grouped by it.
const (
	AnalysisResource     = "resource"
	AnalysisMergeRequest = "merge_request"
	AnalysisTroubleshoot = "troubleshoot"
	AnalysisNamespace    = "namespace"
)

// WithAnalysis marks the model calls made with the returned context as part of
// the named analysis. Pass one of the Analysis constants: the name is sent to
// the telemetry endpoint, so it must never be text taken from a request.
func WithAnalysis(ctx context.Context, analysis string) context.Context {
	return context.WithValue(ctx, analysisKey{}, analysis)
}

func analysisFrom(ctx context.Context) string {
	analysis, _ := ctx.Value(analysisKey{}).(string)
	return analysis
}
