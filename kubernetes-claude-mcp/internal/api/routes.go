package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/argocd"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/upstream"
	"github.com/gorilla/mux"
)

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	// Apply CORS middleware to all routes
	s.router.Use(s.corsMiddleware)

	// API version prefix
	apiV1 := s.router.PathPrefix("/api/v1").Subrouter()

	// Health check endpoints (no auth required)
	apiV1.HandleFunc("/health", s.handleHealth).Methods("GET")
	apiV1.HandleFunc("/health/live", s.handleLiveness).Methods("GET")
	apiV1.HandleFunc("/health/ready", s.handleReadiness).Methods("GET")

	// Add authentication middleware to all other routes
	apiSecure := apiV1.NewRoute().Subrouter()
	apiSecure.Use(s.authMiddleware)

	// MCP endpoints
	apiSecure.HandleFunc("/mcp", s.handleMCPRequest).Methods("POST")
	apiSecure.HandleFunc("/mcp/resource", s.handleResourceQuery).Methods("POST")
	apiSecure.HandleFunc("/mcp/commit", s.handleCommitQuery).Methods("POST")
	apiSecure.HandleFunc("/mcp/troubleshoot", s.handleTroubleshoot).Methods("POST")

	// Kubernetes resource endpoints
	apiSecure.HandleFunc("/namespaces", s.handleListNamespaces).Methods("GET")
	apiSecure.HandleFunc("/resources/{resource}", s.handleListResources).Methods("GET")
	apiSecure.HandleFunc("/resources/{resource}/{name}", s.handleGetResource).Methods("GET")
	apiSecure.HandleFunc("/events", s.handleGetEvents).Methods("GET")

	// ArgoCD endpoints
	apiSecure.HandleFunc("/argocd/applications", s.handleListArgoApplications).Methods("GET")
	apiSecure.HandleFunc("/argocd/applications/{name}", s.handleGetArgoApplication).Methods("GET")

	// GitLab endpoints
	apiSecure.HandleFunc("/gitlab/projects", s.handleListGitLabProjects).Methods("GET")
	apiSecure.HandleFunc("/gitlab/projects/{projectId}/pipelines", s.handleListGitLabPipelines).Methods("GET")

	// Merge Request endpoints
	apiSecure.HandleFunc("/mcp/mergeRequest", s.handleMergeRequestQuery).Methods("POST")
}

// handleMergeRequestQuery handles MCP requests for analyzing merge requests
func (s *Server) handleMergeRequestQuery(w http.ResponseWriter, r *http.Request) {
	var request models.MCPRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	// Force action to be queryMergeRequest
	request.Action = "queryMergeRequest"

	// Validate merge request parameters
	if request.ProjectID == "" || request.MergeRequestIID <= 0 {
		s.respondWithError(w, http.StatusBadRequest, "Project ID and merge request IID are required", nil)
		return
	}

	s.logger.Info("Received merge request query",
		"projectId", request.ProjectID,
		"mergeRequestIID", request.MergeRequestIID)

	// Process the request
	response, err := s.mcpHandler.ProcessRequest(r.Context(), &request)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to process request", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type healthResponse struct {
		Status     string                 `json:"status"`
		Services   map[string]string      `json:"services"`
		Components map[string]interface{} `json:"components"`
	}

	// Check each service
	services := map[string]string{
		"kubernetes": "unknown",
		"argocd":     "unknown",
		"gitlab":     "unknown",
		"claude":     "unknown",
	}

	ctx := r.Context()

	// Check Kubernetes connectivity
	if err := s.k8sClient.CheckConnectivity(ctx); err != nil {
		services["kubernetes"] = "unavailable"
		s.logger.Warn("Kubernetes health check failed", "error", err)
	} else {
		services["kubernetes"] = "available"
	}

	// Check ArgoCD connectivity. Connectivity alone said "available" all
	// through the 2026-09-28 RBAC wipe; the cached authorization probe is what
	// tells a reachable ArgoCD from a usable one.
	argoAccess := s.argoClient.ApplicationsAccess()
	switch err := s.argoClient.CheckConnectivity(ctx); {
	case err != nil:
		services["argocd"] = "unavailable"
		s.logger.Warn("ArgoCD health check failed", "error", err)
	case argoAccess.Refused():
		services["argocd"] = "unauthorized"
	default:
		services["argocd"] = "available"
	}

	// Check GitLab connectivity
	if err := s.gitlabClient.CheckConnectivity(ctx); err != nil {
		services["gitlab"] = "unavailable"
		s.logger.Warn("GitLab health check failed", "error", err)
	} else {
		services["gitlab"] = "available"
	}

	// For Claude, we just assume it's available since we don't want to make an API call
	// in a health check endpoint
	services["claude"] = "assumed available"

	// Determine overall status
	status := "ok"
	if services["kubernetes"] != "available" || argoAccess.Refused() {
		status = "degraded"
	}

	response := healthResponse{
		Status:     status,
		Services:   services,
		Components: map[string]interface{}{"argocd": newArgoCDHealth(argoAccess)},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// handleLiveness handles Kubernetes liveness probe requests
// This endpoint checks if the application is running and should be restarted if it fails
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	type livenessResponse struct {
		Status string `json:"status"`
		Alive  bool   `json:"alive"`
	}

	// Liveness check is simple - if we can respond, we're alive
	response := livenessResponse{
		Status: "ok",
		Alive:  true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// handleReadiness handles Kubernetes readiness probe requests
// This endpoint checks if the application is ready to serve traffic
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	type readinessResponse struct {
		Status     string                 `json:"status"`
		Ready      bool                   `json:"ready"`
		Checks     map[string]bool        `json:"checks"`
		Components map[string]interface{} `json:"components"`
	}

	ctx := r.Context()
	checks := map[string]bool{
		"kubernetes": false,
	}

	// ArgoCD is reported under components, never under checks: it must not
	// gate readiness. A failing readiness probe pulls the pod out of its
	// Service and takes every Kubernetes and GitLab tool down with the ArgoCD
	// ones, so a denied or unreachable ArgoCD degrades one integration instead
	// of the whole server. The value is cached by the argocd client's watch
	// loop, so this adds no ArgoCD call to the probe.
	argoAccess := s.argoClient.ApplicationsAccess()
	components := map[string]interface{}{"argocd": newArgoCDHealth(argoAccess)}

	// Check Kubernetes connectivity - this is critical for readiness
	if err := s.k8sClient.CheckConnectivity(ctx); err != nil {
		s.logger.Debug("Kubernetes readiness check failed", "error", err)
		response := readinessResponse{
			Status:     "not ready",
			Ready:      false,
			Checks:     checks,
			Components: components,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	checks["kubernetes"] = true

	// If Kubernetes is available, we're ready. A definite ArgoCD refusal is a
	// misconfiguration worth surfacing, so it downgrades the status text while
	// the probe itself still passes.
	status := "ready"
	if argoAccess.Refused() {
		status = "degraded"
	}

	response := readinessResponse{
		Status:     status,
		Ready:      true,
		Checks:     checks,
		Components: components,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// argoCDHealth is the argocd entry under "components" in the health responses.
// Authorized is a pointer so "no answer yet" (null) stays distinguishable from a
// definite "no" (false).
type argoCDHealth struct {
	Status     argocd.AuthzState `json:"status"`
	Reachable  bool              `json:"reachable"`
	Authorized *bool             `json:"authorized"`
	CheckedAt  *time.Time        `json:"checkedAt,omitempty"`
	Message    string            `json:"message,omitempty"`
}

func newArgoCDHealth(access argocd.AuthzStatus) argoCDHealth {
	health := argoCDHealth{
		Status:    access.State,
		Reachable: access.Reachable(),
	}
	if access.Reachable() {
		authorized := access.State == argocd.AuthzAuthorized
		health.Authorized = &authorized
	}
	if !access.CheckedAt.IsZero() {
		checkedAt := access.CheckedAt
		health.CheckedAt = &checkedAt
	}
	// The health endpoints are unauthenticated, so only the fixed refusal
	// reasons are echoed here. Transport errors can carry internal URLs and
	// response bodies; those stay in the logs.
	if access.Refused() && access.Err != nil {
		health.Message = access.Err.Error()
	}
	return health
}

// handleMCPRequest handles generic MCP requests
func (s *Server) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	var request models.MCPRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	s.logger.Info("Received MCP request", "action", request.Action)

	// Process the request
	response, err := s.mcpHandler.ProcessRequest(r.Context(), &request)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to process request", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleResourceQuery handles MCP requests for querying resources
func (s *Server) handleResourceQuery(w http.ResponseWriter, r *http.Request) {
	var request models.MCPRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	// Force action to be queryResource
	request.Action = "queryResource"

	// Validate resource parameters
	if request.Resource == "" || request.Name == "" {
		s.respondWithError(w, http.StatusBadRequest, "Resource and name are required", nil)
		return
	}

	s.logger.Info("Received resource query",
		"resource", request.Resource,
		"name", request.Name,
		"namespace", request.Namespace)

	// Special handling for namespace resources to provide comprehensive data
	if strings.EqualFold(request.Resource, "namespace") {
		// Get namespace topology
		topology, err := s.k8sClient.GetNamespaceTopology(r.Context(), request.Name)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "Failed to get namespace topology", err)
			return
		}

		// Get all resources in the namespace
		resources, err := s.k8sClient.GetAllNamespaceResources(r.Context(), request.Name)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "Failed to get namespace resources", err)
			return
		}

		// Get namespace analysis
		analysis, err := s.mcpHandler.AnalyzeNamespace(r.Context(), request.Name)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "Failed to analyze namespace", err)
			return
		}

		// Create an enhanced request with the gathered data
		enhancedRequest := request
		enhancedRequest.Context = fmt.Sprintf("# Namespace Analysis: %s\n\n", request.Name)
		enhancedRequest.Context += "## Resource Counts\n"
		for kind, count := range resources.Stats {
			enhancedRequest.Context += fmt.Sprintf("- %s: %d\n", kind, count)
		}
		enhancedRequest.Context += "\n## Resource Relationships\n"
		for _, rel := range topology.Relationships {
			enhancedRequest.Context += fmt.Sprintf("- %s/%s → %s/%s (%s)\n",
				rel.SourceKind, rel.SourceName, rel.TargetKind, rel.TargetName, rel.RelationType)
		}
		enhancedRequest.Context += "\n## Health Status\n"
		for kind, statuses := range topology.Health {
			healthy := 0
			unhealthy := 0
			progressing := 0
			unknown := 0

			for _, status := range statuses {
				switch status {
				case "healthy":
					healthy++
				case "unhealthy":
					unhealthy++
				case "progressing":
					progressing++
				default:
					unknown++
				}
			}

			enhancedRequest.Context += fmt.Sprintf("- %s: %d healthy, %d unhealthy, %d progressing, %d unknown\n",
				kind, healthy, unhealthy, progressing, unknown)
		}

		// Get events for the namespace
		events, err := s.k8sClient.GetNamespaceEvents(r.Context(), request.Name)
		if err == nil && len(events) > 0 {
			enhancedRequest.Context += "\n## Recent Events\n"
			for i, event := range events {
				if i >= 10 {
					break // Limit to 10 events
				}
				enhancedRequest.Context += fmt.Sprintf("- [%s] %s: %s\n",
					event.Type, event.Reason, event.Message)
			}
		}

		// Process the enhanced request
		response, err := s.mcpHandler.ProcessRequest(r.Context(), &enhancedRequest)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "Failed to process request", err)
			return
		}

		// Add analysis insights to the response
		if analysis != nil {
			response.NamespaceAnalysis = analysis
		}

		s.respondWithJSON(w, http.StatusOK, response)
		return
	}

	// Process regular resource query
	response, err := s.mcpHandler.ProcessRequest(r.Context(), &request)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to process request", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleCommitQuery handles MCP requests for analyzing commits
func (s *Server) handleCommitQuery(w http.ResponseWriter, r *http.Request) {
	var request models.MCPRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	// Force action to be queryCommit
	request.Action = "queryCommit"

	// Validate commit parameters
	if request.ProjectID == "" || request.CommitSHA == "" {
		s.respondWithError(w, http.StatusBadRequest, "Project ID and commit SHA are required", nil)
		return
	}

	s.logger.Info("Received commit query",
		"projectId", request.ProjectID,
		"commitSha", request.CommitSHA)

	// Process the request
	response, err := s.mcpHandler.ProcessRequest(r.Context(), &request)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to process request", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleTroubleshoot handles troubleshooting requests
func (s *Server) handleTroubleshoot(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Resource  string `json:"resource"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Query     string `json:"query,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid request format", err)
		return
	}

	// Validate parameters
	if request.Resource == "" || request.Name == "" {
		s.respondWithError(w, http.StatusBadRequest, "Resource and name are required", nil)
		return
	}

	s.logger.Info("Received troubleshoot request",
		"resource", request.Resource,
		"name", request.Name,
		"namespace", request.Namespace)

	// Process the troubleshooting request
	result, err := s.troubleshootCorrelator.TroubleshootResource(
		r.Context(),
		request.Namespace,
		request.Resource,
		request.Name,
	)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to troubleshoot resource", err)
		return
	}

	// If there's a query, use Claude to analyze the results
	if request.Query != "" {
		mcpRequest := &models.MCPRequest{
			Resource:  request.Resource,
			Name:      request.Name,
			Namespace: request.Namespace,
			Query:     request.Query,
		}

		response, err := s.mcpHandler.ProcessTroubleshootRequest(r.Context(), mcpRequest, result)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "Failed to process troubleshoot analysis", err)
			return
		}

		// Add the troubleshoot result to the response
		responseWithResult := struct {
			*models.MCPResponse
			TroubleshootResult *models.TroubleshootResult `json:"troubleshootResult"`
		}{
			MCPResponse:        response,
			TroubleshootResult: result,
		}

		s.respondWithJSON(w, http.StatusOK, responseWithResult)
		return
	}

	// If no query, just return the troubleshoot result
	s.respondWithJSON(w, http.StatusOK, result)
}

// handleListNamespaces handles requests to list namespaces
func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := s.k8sClient.GetNamespaces(r.Context())
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to list namespaces", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string][]string{"namespaces": namespaces})
}

// handleListResources handles requests to list resources of a specific type
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	resourceType := vars["resource"]
	namespace := r.URL.Query().Get("namespace")

	resources, err := s.k8sClient.ListResources(r.Context(), resourceType, namespace)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to list resources", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"resources": resources})
}

// handleGetResource handles requests to get a specific resource
func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	resourceType := vars["resource"]
	name := vars["name"]
	namespace := r.URL.Query().Get("namespace")

	resource, err := s.k8sClient.GetResource(r.Context(), resourceType, namespace, name)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to get resource", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, resource)
}

// handleGetEvents handles requests to get events
func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	resourceType := r.URL.Query().Get("resource")
	name := r.URL.Query().Get("name")

	events, err := s.k8sClient.GetResourceEvents(r.Context(), namespace, resourceType, name)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to get events", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"events": events})
}

// handleListArgoApplications handles requests to list ArgoCD applications
func (s *Server) handleListArgoApplications(w http.ResponseWriter, r *http.Request) {
	applications, err := s.argoClient.ListApplications(r.Context())
	if err != nil {
		s.respondWithArgoCDError(w, "Failed to list ArgoCD applications", err)
		return
	}

	// ArgoCD filters the list down to the applications the caller may get, so a
	// total RBAC denial arrives as a successful, empty list. Passed through,
	// that reads as "there are no applications": on 2026-09-28 it blanked
	// Meerkat's deployment dashboard for every app for ~16 hours. When the
	// cached probe says ArgoCD refused this token, say so instead. A non-empty
	// list is always returned as-is, and an unknown or unavailable probe keeps
	// the old behavior. (A rejected token is a 401, which ListApplications
	// reports as an error; respondWithArgoCDError answers it the same way.)
	if len(applications) == 0 {
		if access := s.argoClient.ApplicationsAccess(); access.Refused() {
			s.respondWithError(w, http.StatusServiceUnavailable, argoCDRefusedMessage, access.Err)
			return
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"applications": applications})
}

// handleGetArgoApplication handles requests to get a specific ArgoCD application
func (s *Server) handleGetArgoApplication(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	application, err := s.argoClient.GetApplication(r.Context(), name)
	if err != nil {
		s.respondWithArgoCDError(w, "Failed to get ArgoCD application", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, application)
}

// argoCDRefusedMessage is the error for every response that reports ArgoCD
// refusing this server's token, whether the probe or the call itself said so.
const argoCDRefusedMessage = "ArgoCD denied applications:get for this server's token"

// respondWithArgoCDError answers a failed ArgoCD call. A 401 means ArgoCD
// rejected this server's token: the refusal the authorization probe reports
// as unauthenticated, so it gets the same 503 and fixed remediation text as
// the probe path rather than a generic 500. Everything else is a 500.
func (s *Server) respondWithArgoCDError(w http.ResponseWriter, message string, err error) {
	if upstream.StatusCode(err) == http.StatusUnauthorized {
		s.respondWithError(w, http.StatusServiceUnavailable, argoCDRefusedMessage, argocd.ErrTokenRejected)
		return
	}
	s.respondWithError(w, http.StatusInternalServerError, message, err)
}

// handleListGitLabProjects handles requests to list GitLab projects
func (s *Server) handleListGitLabProjects(w http.ResponseWriter, r *http.Request) {
	// This would typically include pagination parameters
	projects, err := s.gitlabClient.ListProjects(r.Context())
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to list GitLab projects", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})
}

// handleListGitLabPipelines handles requests to list GitLab pipelines
func (s *Server) handleListGitLabPipelines(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	projectId := vars["projectId"]

	pipelines, err := s.gitlabClient.ListPipelines(r.Context(), projectId)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to list GitLab pipelines", err)
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"pipelines": pipelines})
}

// Helper methods

// respondWithError sends an error response to the client
func (s *Server) respondWithError(w http.ResponseWriter, code int, message string, err error) {
	errorResponse := map[string]string{
		"error": message,
	}

	if err != nil {
		errorResponse["details"] = err.Error()
		s.logger.Error(message, "error", err, "code", code)
	} else {
		s.logger.Warn(message, "code", code)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errorResponse)
}

// respondWithJSON sends a JSON response to the client
//
//nolint:unparam // code parameter is kept for consistency with error response pattern
func (s *Server) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
