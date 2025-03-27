package api

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"

    "github.com/gorilla/mux"
    "github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	
	// API version prefix
	apiV1 := s.router.PathPrefix("/api/v1").Subrouter()
	
	// Health check endpoint (no auth required)
	apiV1.HandleFunc("/health", s.handleHealth).Methods("GET")
	
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
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type healthResponse struct {
		Status string `json:"status"`
		Services map[string]string `json:"services"`
	}
	
	// Check each service
	services := map[string]string{
		"kubernetes": "unknown",
		"argocd": "unknown",
		"gitlab": "unknown",
		"claude": "unknown",
	}
	
	ctx := r.Context()
	
	// Check Kubernetes connectivity
	if err := s.k8sClient.CheckConnectivity(ctx); err != nil {
		services["kubernetes"] = "unavailable"
		s.logger.Warn("Kubernetes health check failed", "error", err)
	} else {
		services["kubernetes"] = "available"
	}
	
	// Check ArgoCD connectivity
	if err := s.argoClient.CheckConnectivity(ctx); err != nil {
		services["argocd"] = "unavailable"
		s.logger.Warn("ArgoCD health check failed", "error", err)
	} else {
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
	if services["kubernetes"] != "available" {
		status = "degraded"
	}
	
	response := healthResponse{
		Status: status,
		Services: services,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
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
    if strings.ToLower(request.Resource) == "namespace" {
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
        enhancedRequest.Context += fmt.Sprintf("## Resource Counts\n")
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
		s.respondWithError(w, http.StatusInternalServerError, "Failed to list ArgoCD applications", err)
		return
	}
	
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"applications": applications})
}

// handleGetArgoApplication handles requests to get a specific ArgoCD application
func (s *Server) handleGetArgoApplication(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]
	
	application, err := s.argoClient.GetApplication(r.Context(), name)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to get ArgoCD application", err)
		return
	}
	
	s.respondWithJSON(w, http.StatusOK, application)
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
	json.NewEncoder(w).Encode(errorResponse)
}

// respondWithJSON sends a JSON response to the client
func (s *Server) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}