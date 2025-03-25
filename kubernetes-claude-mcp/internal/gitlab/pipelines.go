package gitlab

import (
	"io"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// ListPipelines returns a list of pipelines for a project
func (c *Client) ListPipelines(ctx context.Context, projectID string) ([]models.GitLabPipeline, error) {
	c.logger.Debug("Listing pipelines", "projectID", projectID)
	
	endpoint := fmt.Sprintf("projects/%s/pipelines", url.PathEscape(projectID))
	
	// Add query parameters for pagination
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}
	
	q := u.Query()
	q.Set("per_page", "20")
	q.Set("order_by", "id")
	q.Set("sort", "desc")
	u.RawQuery = q.Encode()
	
	resp, err := c.doRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pipelines []models.GitLabPipeline
	if err := json.NewDecoder(resp.Body).Decode(&pipelines); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Listed pipelines", "projectID", projectID, "count", len(pipelines))
	return pipelines, nil
}

// GetPipeline returns details about a specific pipeline
func (c *Client) GetPipeline(ctx context.Context, projectID string, pipelineID int) (*models.GitLabPipeline, error) {
	c.logger.Debug("Getting pipeline", "projectID", projectID, "pipelineID", pipelineID)
	
	endpoint := fmt.Sprintf("projects/%s/pipelines/%d", url.PathEscape(projectID), pipelineID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pipeline models.GitLabPipeline
	if err := json.NewDecoder(resp.Body).Decode(&pipeline); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &pipeline, nil
}

// GetPipelineJobs returns jobs for a specific pipeline
func (c *Client) GetPipelineJobs(ctx context.Context, projectID string, pipelineID int) ([]models.GitLabJob, error) {
	c.logger.Debug("Getting pipeline jobs", "projectID", projectID, "pipelineID", pipelineID)
	
	endpoint := fmt.Sprintf("projects/%s/pipelines/%d/jobs", url.PathEscape(projectID), pipelineID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jobs []models.GitLabJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Got pipeline jobs", "projectID", projectID, "pipelineID", pipelineID, "count", len(jobs))
	return jobs, nil
}

// FindRecentDeployments finds recent deployments to a specific environment
func (c *Client) FindRecentDeployments(ctx context.Context, projectID, environment string) ([]models.GitLabDeployment, error) {
	c.logger.Debug("Finding recent deployments", 
		"projectID", projectID, 
		"environment", environment)
	
	// Create endpoint with query parameters
	endpoint := fmt.Sprintf("projects/%s/deployments", url.PathEscape(projectID))
	
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}
	
	q := u.Query()
	q.Set("environment", environment)
	q.Set("order_by", "created_at")
	q.Set("sort", "desc")
	q.Set("per_page", "10")
	u.RawQuery = q.Encode()
	
	resp, err := c.doRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var deployments []models.GitLabDeployment
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Found deployments", 
		"projectID", projectID, 
		"environment", environment, 
		"count", len(deployments))
	return deployments, nil
}

// GetJobLogs retrieves logs for a specific job
func (c *Client) GetJobLogs(ctx context.Context, projectID string, jobID int) (string, error) {
	c.logger.Debug("Getting job logs", "projectID", projectID, "jobID", jobID)
	
	endpoint := fmt.Sprintf("projects/%s/jobs/%d/trace", url.PathEscape(projectID), jobID)
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	logs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(logs), nil
}