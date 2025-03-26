package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

// setupNamespaceRoutes configures the API routes for namespace analysis
func (s *Server) setupNamespaceRoutes() {
	// Add to the secure API subrouter
	apiSecure := s.router.PathPrefix("/api/v1").Subrouter()
	apiSecure.Use(s.authMiddleware)
	
	// Namespace analysis endpoints
	apiSecure.HandleFunc("/namespaces/{namespace}/topology", s.handleNamespaceTopology).Methods("GET")
	apiSecure.HandleFunc("/namespaces/{namespace}/graph", s.handleNamespaceGraph).Methods("GET")
	apiSecure.HandleFunc("/namespaces/{namespace}/resources", s.handleNamespaceResources).Methods("GET")
	apiSecure.HandleFunc("/namespaces/{namespace}/analysis", s.handleNamespaceAnalysis).Methods("GET")
}

// handleNamespaceTopology handles requests for namespace topology information
func (s *Server) handleNamespaceTopology(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	namespace := vars["namespace"]
	
	s.logger.Info("Handling namespace topology request", "namespace", namespace)
	
	// Get topology from the resource mapper
	topology, err := s.resourceMapper.GetNamespaceTopology(r.Context(), namespace)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to get namespace topology", err)
		return
	}
	
	s.respondWithJSON(w, http.StatusOK, topology)
}

// handleNamespaceGraph handles requests for namespace resource graph
func (s *Server) handleNamespaceGraph(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	namespace := vars["namespace"]
	
	s.logger.Info("Handling namespace graph request", "namespace", namespace)
	
	// Get resource graph from the resource mapper
	graph, err := s.resourceMapper.GetResourceGraph(r.Context(), namespace)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to get namespace graph", err)
		return
	}
	
	s.respondWithJSON(w, http.StatusOK, graph)
}

// handleNamespaceResources handles requests for namespace resources
func (s *Server) handleNamespaceResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	namespace := vars["namespace"]
	
	s.logger.Info("Handling namespace resources request", "namespace", namespace)
	
	// Get all resources in the namespace
	resources, err := s.k8sClient.GetAllNamespaceResources(r.Context(), namespace)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to get namespace resources", err)
		return
	}
	
	s.respondWithJSON(w, http.StatusOK, resources)
}

// handleNamespaceAnalysis handles requests for namespace analysis
func (s *Server) handleNamespaceAnalysis(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	namespace := vars["namespace"]
	
	s.logger.Info("Handling namespace analysis request", "namespace", namespace)
	
	// Get namespace analysis from the MCP protocol handler
	analysis, err := s.mcpHandler.AnalyzeNamespace(r.Context(), namespace)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "Failed to analyze namespace", err)
		return
	}
	
	s.respondWithJSON(w, http.StatusOK, analysis)
}