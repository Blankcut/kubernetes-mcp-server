package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// ListApplications returns a list of all ArgoCD applications
func (c *Client) ListApplications(ctx context.Context) ([]models.ArgoApplication, error) {
	c.logger.Debug("Listing ArgoCD applications")

	// Try the v1 API path
	endpoint := "/api/v1/applications"
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Items []models.ArgoApplication `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Listed ArgoCD applications", "count", len(result.Items))
	return result.Items, nil
}

// GetApplication returns details about a specific ArgoCD application
func (c *Client) GetApplication(ctx context.Context, name string) (*models.ArgoApplication, error) {
	c.logger.Debug("Getting ArgoCD application", "name", name)

	endpoint := fmt.Sprintf("/api/v1/applications/%s", url.PathEscape(name))
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var app models.ArgoApplication
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &app, nil
}

// GetResourceTree returns the resource hierarchy for an application
func (c *Client) GetResourceTree(ctx context.Context, name string) (*models.ArgoResourceTree, error) {
	c.logger.Debug("Getting resource tree for application", "name", name)

	endpoint := fmt.Sprintf("/api/v1/applications/%s/resource-tree", url.PathEscape(name))
	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tree models.ArgoResourceTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Retrieved resource tree", "name", name, "nodeCount", len(tree.Nodes))
	return &tree, nil
}

// FindApplicationsByResource finds all ArgoCD applications that manage a specific Kubernetes resource
func (c *Client) FindApplicationsByResource(ctx context.Context, kind, name, namespace string) ([]models.ArgoApplication, error) {
	c.logger.Debug("Finding applications by resource",
		"kind", kind,
		"name", name,
		"namespace", namespace)

	// First try to use the resource API endpoint if available
	endpoint := fmt.Sprintf("/api/v1/applications/resource/%s/%s/%s/%s/%s",
		url.PathEscape(""),
		url.PathEscape(kind),
		url.PathEscape(namespace),
		url.PathEscape(name),
		url.PathEscape(""),
	)

	resp, err := c.doRequest(ctx, http.MethodGet, endpoint, nil)
	if err == nil {
		defer resp.Body.Close()

		var appRefs []struct {
			Name string `json:"name"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&appRefs); err != nil {
			c.logger.Warn("Failed to decode application references", "error", err)
		} else if len(appRefs) > 0 {
			// Get full application details for each reference
			var apps []models.ArgoApplication
			for _, ref := range appRefs {
				app, err := c.GetApplication(ctx, ref.Name)
				if err != nil {
					c.logger.Warn("Failed to get application details",
						"name", ref.Name,
						"error", err)
					continue
				}
				apps = append(apps, *app)
			}

			c.logger.Debug("Found applications by resource API",
				"resourceKind", kind,
				"resourceName", name,
				"count", len(apps))
			return apps, nil
		}
	}

	// Fallback: Get all applications and check their resource trees
	c.logger.Debug("Resource API failed, falling back to application scanning")
	apps, err := c.ListApplications(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}

	var matchingApps []models.ArgoApplication

	// For each application, check if it manages the specified resource
	for _, app := range apps {
		tree, err := c.GetResourceTree(ctx, app.Name)
		if err != nil {
			c.logger.Warn("Failed to get resource tree",
				"application", app.Name,
				"error", err)
			continue // Skip this app if we can't get its resource tree
		}

		for _, node := range tree.Nodes {
			// Match against the specified resource
			if strings.EqualFold(node.Kind, kind) &&
				node.Name == name &&
				(namespace == "" || node.Namespace == namespace) {
				matchingApps = append(matchingApps, app)
				break // Found a match in this app, move to the next app
			}
		}
	}

	c.logger.Debug("Found applications managing resource by scanning",
		"resourceKind", kind,
		"resourceName", name,
		"count", len(matchingApps))
	return matchingApps, nil
}
