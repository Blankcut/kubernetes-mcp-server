package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// ListProjects returns a list of GitLab projects
func (c *Client) ListProjects(ctx context.Context) ([]models.GitLabProject, error) {
	c.logger.Debug("Listing projects")

	// Create endpoint with query parameters
	endpoint := "projects"

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	q := u.Query()
	q.Set("membership", "true")
	q.Set("order_by", "updated_at")
	q.Set("sort", "desc")
	q.Set("per_page", "100")
	u.RawQuery = q.Encode()

	resp, err := c.doRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var projects []models.GitLabProject
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Listed projects", "count", len(projects))
	return projects, nil
}

// GetProject returns details about a specific GitLab project
func (c *Client) GetProject(ctx context.Context, projectID string) (*models.GitLabProject, error) {
	c.logger.Debug("Getting project", "projectID", projectID)

	endpoint := fmt.Sprintf("projects/%s", url.PathEscape(projectID))
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var project models.GitLabProject
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &project, nil
}

// GetProjectByPath returns a project by its path (namespace/project-name)
func (c *Client) GetProjectByPath(ctx context.Context, path string) (*models.GitLabProject, error) {
	c.logger.Debug("Getting project by path", "path", path)

	// GitLab API requires path to be URL encoded
	encodedPath := url.QueryEscape(path)
	endpoint := fmt.Sprintf("projects/%s", encodedPath)

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var project models.GitLabProject
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &project, nil
}

// GetCommit returns details about a specific commit
func (c *Client) GetCommit(ctx context.Context, projectID, sha string) (*models.GitLabCommit, error) {
	c.logger.Debug("Getting commit", "projectID", projectID, "sha", sha)

	endpoint := fmt.Sprintf("projects/%s/repository/commits/%s", url.PathEscape(projectID), url.PathEscape(sha))
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var commit models.GitLabCommit
	if err := json.NewDecoder(resp.Body).Decode(&commit); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &commit, nil
}

// GetCommitDiff returns the changes in a specific commit
func (c *Client) GetCommitDiff(ctx context.Context, projectID, sha string) ([]models.GitLabDiff, error) {
	c.logger.Debug("Getting commit diff", "projectID", projectID, "sha", sha)

	endpoint := fmt.Sprintf("projects/%s/repository/commits/%s/diff", url.PathEscape(projectID), url.PathEscape(sha))
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var diffs []models.GitLabDiff
	if err := json.NewDecoder(resp.Body).Decode(&diffs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Got commit diff", "projectID", projectID, "sha", sha, "count", len(diffs))
	return diffs, nil
}

// GetFileContent returns the content of a file at a specific commit
func (c *Client) GetFileContent(ctx context.Context, projectID, filePath, ref string) (string, error) {
	c.logger.Debug("Getting file content",
		"projectID", projectID,
		"filePath", filePath,
		"ref", ref)

	encodedFilePath := url.PathEscape(filePath)
	endpoint := fmt.Sprintf("projects/%s/repository/files/%s/raw",
		url.PathEscape(projectID),
		encodedFilePath)

	// Add ref parameter if provided
	if ref != "" {
		u, err := url.Parse(endpoint)
		if err != nil {
			return "", fmt.Errorf("invalid endpoint: %w", err)
		}

		q := u.Query()
		q.Set("ref", ref)
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read file content: %w", err)
	}

	return string(content), nil
}

// FindRecentChanges finds recent changes (commits) for a project
func (c *Client) FindRecentChanges(ctx context.Context, projectID string, since time.Time) ([]models.GitLabCommit, error) {
	c.logger.Debug("Finding recent changes",
		"projectID", projectID,
		"since", since.Format(time.RFC3339))

	// Format time as ISO 8601
	sinceStr := since.Format(time.RFC3339)

	// Create endpoint with query parameters
	endpoint := fmt.Sprintf("projects/%s/repository/commits", url.PathEscape(projectID))

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	q := u.Query()
	q.Set("since", sinceStr)
	q.Set("per_page", "20")
	u.RawQuery = q.Encode()

	resp, err := c.doRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var commits []models.GitLabCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Found recent changes",
		"projectID", projectID,
		"count", len(commits))
	return commits, nil
}
