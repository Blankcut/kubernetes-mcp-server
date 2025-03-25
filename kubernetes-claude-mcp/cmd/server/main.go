package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/api"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/argocd"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/claude"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/correlator"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/gitlab"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/mcp"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

func main() {

	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "path to config file")
	logLevel := flag.String("log-level", "info", "logging level (debug, info, warn, error)")
	flag.Parse()

	// Initialize logger
	os.Setenv("LOG_LEVEL", *logLevel)
	logger := logging.NewLogger()
	logger.Info("Starting Kubernetes Claude MCP server")

	// Load configuration
	logger.Info("Loading configuration", "path", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration", "error", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", "error", err)
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize credential provider
	logger.Info("Initializing credential provider")
	credProvider := auth.NewCredentialProvider(cfg)
	if err := credProvider.LoadCredentials(ctx); err != nil {
		logger.Fatal("Failed to load credentials", "error", err)
	}

	// Initialize Kubernetes client
	logger.Info("Initializing Kubernetes client")
	k8sClient, err := k8s.NewClient(cfg.Kubernetes, logger.Named("k8s"))
	if err != nil {
		logger.Fatal("Failed to create Kubernetes client", "error", err)
	}

	// Check Kubernetes connectivity
	if err := k8sClient.CheckConnectivity(ctx); err != nil {
		logger.Warn("Kubernetes connectivity check failed", "error", err)
	} else {
		logger.Info("Kubernetes connectivity confirmed")
	}

	// Initialize ArgoCD client
	logger.Info("Initializing ArgoCD client")
	argoClient := argocd.NewClient(&cfg.ArgoCD, credProvider, logger.Named("argocd"))

	// Check ArgoCD connectivity (don't fail if unavailable)
	if err := argoClient.CheckConnectivity(ctx); err != nil {
		logger.Warn("ArgoCD connectivity check failed", "error", err)
	} else {
		logger.Info("ArgoCD connectivity confirmed")
	}

	// Initialize GitLab client
	logger.Info("Initializing GitLab client")
	gitlabClient := gitlab.NewClient(&cfg.GitLab, credProvider, logger.Named("gitlab"))

	// Check GitLab connectivity (don't fail if unavailable)
	if err := gitlabClient.CheckConnectivity(ctx); err != nil {
		logger.Warn("GitLab connectivity check failed", "error", err)
	} else {
		logger.Info("GitLab connectivity confirmed")
	}

	// Initialize Claude client
	logger.Info("Initializing Claude client")
	claudeConfig := claude.ClaudeConfig{
		APIKey:      cfg.Claude.APIKey,
		BaseURL:     cfg.Claude.BaseURL,
		ModelID:     cfg.Claude.ModelID,
		MaxTokens:   cfg.Claude.MaxTokens,
		Temperature: cfg.Claude.Temperature,
	}
	claudeClient := claude.NewClient(claudeConfig, logger.Named("claude"))

	// Initialize GitOps correlator
	logger.Info("Initializing GitOps correlator")
	gitOpsCorrelator := correlator.NewGitOpsCorrelator(
		k8sClient, 
		argoClient, 
		gitlabClient, 
		logger.Named("correlator"),
	)

	// Initialize troubleshoot correlator
	troubleshootCorrelator := correlator.NewTroubleshootCorrelator(
		gitOpsCorrelator, 
		k8sClient,
		logger.Named("troubleshoot"),
	)

	// Initialize MCP protocol handler
	logger.Info("Initializing MCP protocol handler")
	mcpHandler := mcp.NewProtocolHandler(
		claudeClient, 
		gitOpsCorrelator,
		logger.Named("mcp"),
	)

	// Initialize API server
	logger.Info("Initializing API server")
	server := api.NewServer(
		cfg.Server, 
		k8sClient, 
		argoClient, 
		gitlabClient, 
		mcpHandler,
		troubleshootCorrelator,
		logger.Named("api"),
	)

	// Handle graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("Received shutdown signal", "signal", sig)
		
		// Create a timeout context for shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		
		logger.Info("Shutting down server...")
		cancel() // Cancel the main context
		
		// Wait for server to shut down or timeout
		<-shutdownCtx.Done()
	}()

	// Start server
	logger.Info("Starting MCP server", "address", cfg.Server.Address)
	if err := server.Start(ctx); err != nil {
		logger.Fatal("Server error", "error", err)
	}

	logger.Info("Server shutdown complete")
}