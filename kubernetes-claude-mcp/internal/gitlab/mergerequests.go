// internal/gitlab/mergerequests.go

package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// ListMergeRequests returns a list of merge requests for a project
func (c *Client) ListMergeRequests(ctx context.Context, projectID, state string) ([]models.GitLabMergeRequest, error) {
	c.logger.Debug("Listing merge requests", "projectID", projectID, "state", state)

	// Create endpoint with query parameters
	endpoint := fmt.Sprintf("projects/%s/merge_requests", url.PathEscape(projectID))

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	q := u.Query()
	if state != "" {
		q.Set("state", state)
	}
	q.Set("order_by", "updated_at")
	q.Set("sort", "desc")
	q.Set("per_page", "20")
	u.RawQuery = q.Encode()

	resp, err := c.doRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var mergeRequests []models.GitLabMergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mergeRequests); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Listed merge requests", "projectID", projectID, "count", len(mergeRequests))
	return mergeRequests, nil
}

// GetMergeRequest returns details about a specific merge request
func (c *Client) GetMergeRequest(ctx context.Context, projectID string, mergeRequestIID int) (*models.GitLabMergeRequest, error) {
	c.logger.Debug("Getting merge request", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d", url.PathEscape(projectID), mergeRequestIID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var mergeRequest models.GitLabMergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mergeRequest); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &mergeRequest, nil
}

// GetMergeRequestChanges returns the changes in a specific merge request
func (c *Client) GetMergeRequestChanges(ctx context.Context, projectID string, mergeRequestIID int) (*models.GitLabMergeRequest, error) {
	c.logger.Debug("Getting merge request changes", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/changes", url.PathEscape(projectID), mergeRequestIID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var mergeRequest models.GitLabMergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mergeRequest); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &mergeRequest, nil
}

// GetMergeRequestApprovals returns approval information for a merge request
func (c *Client) GetMergeRequestApprovals(ctx context.Context, projectID string, mergeRequestIID int) (*models.GitLabMergeRequestApproval, error) {
	c.logger.Debug("Getting merge request approvals", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/approvals", url.PathEscape(projectID), mergeRequestIID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var approvals models.GitLabMergeRequestApproval
	if err := json.NewDecoder(resp.Body).Decode(&approvals); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &approvals, nil
}

// GetMergeRequestComments returns comments on a merge request
func (c *Client) GetMergeRequestComments(ctx context.Context, projectID string, mergeRequestIID int) ([]models.GitLabMergeRequestComment, error) {
	c.logger.Debug("Getting merge request comments", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/notes", url.PathEscape(projectID), mergeRequestIID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var comments []models.GitLabMergeRequestComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Got merge request comments", "projectID", projectID, "mergeRequestIID", mergeRequestIID, "count", len(comments))
	return comments, nil
}

// GetMergeRequestCommits returns the commits in a merge request
func (c *Client) GetMergeRequestCommits(ctx context.Context, projectID string, mergeRequestIID int) ([]models.GitLabCommit, error) {
	c.logger.Debug("Getting merge request commits", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/commits", url.PathEscape(projectID), mergeRequestIID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var commits []models.GitLabCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Got merge request commits", "projectID", projectID, "mergeRequestIID", mergeRequestIID, "count", len(commits))
	return commits, nil
}

// AnalyzeMergeRequest analyzes a merge request for Kubernetes/Helm changes
func (c *Client) AnalyzeMergeRequest(ctx context.Context, projectID string, mergeRequestIID int) (*models.GitLabMergeRequest, error) {
	c.logger.Debug("Analyzing merge request", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	// Get basic merge request data
	mr, err := c.GetMergeRequest(ctx, projectID, mergeRequestIID)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request: %w", err)
	}

	// Get changes
	mrChanges, err := c.GetMergeRequestChanges(ctx, projectID, mergeRequestIID)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request changes: %w", err)
	}

	// Copy changes to the original merge request
	mr.Changes = mrChanges.Changes

	// Initialize context analysis
	mr.MergeRequestContext.AffectedFiles = make([]string, 0)
	mr.MergeRequestContext.HelmChartAffected = false
	mr.MergeRequestContext.KubernetesManifest = false

	// Analyze changes
	for _, change := range mr.Changes {
		mr.MergeRequestContext.AffectedFiles = append(mr.MergeRequestContext.AffectedFiles, change.NewPath)

		// Check for Helm charts
		if strings.Contains(change.NewPath, "Chart.yaml") ||
			strings.Contains(change.NewPath, "values.yaml") ||
			(strings.Contains(change.NewPath, "templates/") && strings.HasSuffix(change.NewPath, ".yaml")) {
			mr.MergeRequestContext.HelmChartAffected = true
		}

		// Check for Kubernetes manifests
		if strings.HasSuffix(change.NewPath, ".yaml") || strings.HasSuffix(change.NewPath, ".yml") {
			// Look for Kubernetes kind in the file content
			if strings.Contains(change.Diff, "kind:") &&
				(strings.Contains(change.Diff, "Deployment") ||
					strings.Contains(change.Diff, "Service") ||
					strings.Contains(change.Diff, "ConfigMap") ||
					strings.Contains(change.Diff, "Secret") ||
					strings.Contains(change.Diff, "Pod")) {
				mr.MergeRequestContext.KubernetesManifest = true
			}
		}
	}

	// Get commits
	commits, err := c.GetMergeRequestCommits(ctx, projectID, mergeRequestIID)
	if err != nil {
		c.logger.Warn("Failed to get merge request commits", "error", err)
	} else {
		// Extract commit messages
		mr.MergeRequestContext.CommitMessages = make([]string, 0)
		for _, commit := range commits {
			mr.MergeRequestContext.CommitMessages = append(mr.MergeRequestContext.CommitMessages, commit.Title)
		}
	}

	return mr, nil
}

// CreateMergeRequestComment creates a new comment on a merge request
func (c *Client) CreateMergeRequestComment(ctx context.Context, projectID string, mergeRequestIID int, body string) (*models.GitLabMergeRequestComment, error) {
	c.logger.Debug("Creating merge request comment", "projectID", projectID, "mergeRequestIID", mergeRequestIID)

	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/notes", url.PathEscape(projectID), mergeRequestIID)

	// Create request payload
	reqBody := map[string]string{
		"body": body,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, endpoint, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var comment models.GitLabMergeRequestComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &comment, nil
}
