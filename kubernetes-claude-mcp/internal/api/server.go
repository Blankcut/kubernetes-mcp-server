package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/argocd"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/correlator"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/gitlab"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/mcp"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// Server represents the API server
type Server struct {
	router                *mux.Router
	server                *http.Server
	k8sClient             *k8s.Client
	argoClient            *argocd.Client
	gitlabClient          *gitlab.Client
	mcpHandler            *mcp.ProtocolHandler
	troubleshootCorrelator *correlator.TroubleshootCorrelator
	config                config.ServerConfig
	logger                *logging.Logger
}

// NewServer creates a new API server
func NewServer(
	cfg config.ServerConfig,
	k8sClient *k8s.Client,
	argoClient *argocd.Client,
	gitlabClient *gitlab.Client,
	mcpHandler *mcp.ProtocolHandler,
	troubleshootCorrelator *correlator.TroubleshootCorrelator,
	logger *logging.Logger,
) *Server {
	if logger == nil {
		logger = logging.NewLogger().Named("api")
	}
	
	server := &Server{
		router:                mux.NewRouter(),
		k8sClient:             k8sClient,
		argoClient:            argoClient,
		gitlabClient:          gitlabClient,
		mcpHandler:            mcpHandler,
		troubleshootCorrelator: troubleshootCorrelator,
		config:                cfg,
		logger:                logger,
	}
	
	// Set up routes
	server.setupRoutes()
	
	return server
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	s.server = &http.Server{
		Addr:         s.config.Address,
		Handler:      s.loggingMiddleware(s.router),
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
	}
	
	// Channel for server errors
	errCh := make(chan error, 1)
	
	// Start server in a goroutine
	go func() {
		s.logger.Info("Starting HTTP server", "address", s.config.Address)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	
	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("Context cancelled, shutting down server")
		return s.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")
	return s.server.Shutdown(ctx)
}

// Middleware functions

// loggingMiddleware logs information about each request
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a response writer that captures status code
		rw := &responseWriter{w, http.StatusOK}
		
		// Call the next handler
		next.ServeHTTP(rw, r)
		
		// Log the request
		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}

// authMiddleware checks for valid authentication
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get API key from header
		apiKey := r.Header.Get("X-API-Key")
		
		// Check for bearer token if API key is not provided
		if apiKey == "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				s.respondWithError(w, http.StatusUnauthorized, "Authentication required", nil)
				return
			}
			
			// Extract token
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				s.respondWithError(w, http.StatusUnauthorized, "Invalid authorization format", nil)
				return
			}
			
			apiKey = parts[1]
		}
		
		// Validate the API key against the configured key
		if apiKey != s.config.Auth.APIKey {
			s.respondWithError(w, http.StatusUnauthorized, "Invalid API key", nil)
			return
		}
		
		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// Custom response writer to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}