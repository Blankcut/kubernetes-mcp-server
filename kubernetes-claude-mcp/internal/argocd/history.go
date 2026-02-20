package argocd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// GetApplicationHistory returns the deployment history for an application
func (c *Client) GetApplicationHistory(ctx context.Context, name string) ([]models.ArgoApplicationHistory, error) {
	c.logger.Debug("Getting application history", "name", name)

	endpoint := fmt.Sprintf("/api/v1/applications/%s/history", url.PathEscape(name))
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		History []models.ArgoApplicationHistory `json:"history"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Retrieved application history", "name", name, "entryCount", len(result.History))
	return result.History, nil
}

// GetApplicationLogs retrieves logs for a specific application component
func (c *Client) GetApplicationLogs(ctx context.Context, name, podName, containerName string) ([]string, error) {
	c.logger.Debug("Getting application logs",
		"application", name,
		"pod", podName,
		"container", containerName)

	endpoint := fmt.Sprintf("/api/v1/applications/%s/pods/%s/logs?container=%s",
		url.PathEscape(name),
		url.PathEscape(podName),
		url.QueryEscape(containerName))

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// ArgoCD returns logs as a newline-separated string here...
	logEntries := strings.Split(string(body), "\n")
	var logs []string
	for _, entry := range logEntries {
		if entry != "" {
			logs = append(logs, entry)
		}
	}

	c.logger.Debug("Retrieved application logs",
		"application", name,
		"pod", podName,
		"container", containerName,
		"lineCount", len(logs))
	return logs, nil
}

// GetApplicationRevisionMetadata gets metadata about a specific revision
func (c *Client) GetApplicationRevisionMetadata(ctx context.Context, name, revision string) (*models.GitLabCommit, error) {
	c.logger.Debug("Getting revision metadata", "application", name, "revision", revision)

	endpoint := fmt.Sprintf("/api/v1/applications/%s/revisions/%s/metadata",
		url.PathEscape(name),
		url.PathEscape(revision))

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result models.GitLabCommit
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// SyncApplication triggers a sync operation for an application
func (c *Client) SyncApplication(ctx context.Context, name string, revision string, prune bool, resources []string) error {
	c.logger.Debug("Syncing application",
		"name", name,
		"revision", revision,
		"prune", prune)

	// Prepare sync request body
	syncRequest := struct {
		Revision  string   `json:"revision,omitempty"`
		Prune     bool     `json:"prune"`
		Resources []string `json:"resources,omitempty"`
		DryRun    bool     `json:"dryRun"`
	}{
		Revision:  revision,
		Prune:     prune,
		Resources: resources,
		DryRun:    false,
	}

	jsonBody, err := json.Marshal(syncRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal sync request: %w", err)
	}

	endpoint := fmt.Sprintf("/api/v1/applications/%s/sync", url.PathEscape(name))
	resp, err := c.doRequest(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response but we won't need to process it
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Info("Application sync initiated", "name", name)
	return nil
}
