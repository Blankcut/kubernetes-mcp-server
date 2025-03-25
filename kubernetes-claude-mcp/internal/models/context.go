package models

// ResourceContext combines information about a Kubernetes resource with GitOps context
type ResourceContext struct {
	// Basic resource information
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	APIVersion  string `json:"apiVersion"`
	
	// Related ArgoCD information
	ArgoApplication     *ArgoApplication       `json:"argoApplication,omitempty"`
	ArgoSyncStatus      string                 `json:"argoSyncStatus,omitempty"`
	ArgoHealthStatus    string                 `json:"argoHealthStatus,omitempty"`
	ArgoSyncHistory     []ArgoApplicationHistory `json:"argoSyncHistory,omitempty"`
	
	// Related GitLab information
	GitLabProject       *GitLabProject         `json:"gitlabProject,omitempty"`
	LastPipeline        *GitLabPipeline        `json:"lastPipeline,omitempty"`
	LastDeployment      *GitLabDeployment      `json:"lastDeployment,omitempty"`
	RecentCommits       []GitLabCommit         `json:"recentCommits,omitempty"`
	
	// Additional context
	Events              []K8sEvent             `json:"events,omitempty"`
	RelatedResources    []string               `json:"relatedResources,omitempty"`
	Errors              []string               `json:"errors,omitempty"`
}

// Issue represents a discovered issue or potential problem
type Issue struct {
	Title       string `json:"title"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

// TroubleshootResult contains troubleshooting findings and recommendations
type TroubleshootResult struct {
	ResourceContext ResourceContext `json:"resourceContext"`
	Issues          []Issue         `json:"issues"`
	Recommendations []string        `json:"recommendations"`
}

// MCPRequest represents a request to the MCP server
type MCPRequest struct {
	Action        string                 `json:"action"`
	Resource      string                 `json:"resource,omitempty"`
	Namespace     string                 `json:"namespace,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Query         string                 `json:"query,omitempty"`
	CommitSHA     string                 `json:"commitSha,omitempty"`
	ProjectID     string                 `json:"projectId,omitempty"`
	ResourceSpecs map[string]interface{} `json:"resourceSpecs,omitempty"`
	Context       string                 `json:"context,omitempty"`
}

// MCPResponse represents a response from the MCP server
type MCPResponse struct {
    Success           bool                `json:"success"`
    Message           string              `json:"message,omitempty"`
    Analysis          string              `json:"analysis,omitempty"`
    Context           ResourceContext     `json:"context,omitempty"`
    Actions           []string            `json:"actions,omitempty"`
    ErrorDetails      string              `json:"errorDetails,omitempty"`
    TroubleshootResult *TroubleshootResult `json:"troubleshootResult,omitempty"`
}