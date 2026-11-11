package api

import (
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/mcptransport"
)

// setupMCPRoute mounts the Model Context Protocol endpoint (Streamable HTTP)
// for MCP clients, at /mcp.
//
// Not under /api/v1: /api/v1/mcp is the REST analysis route, which predates
// this and is a different thing. It takes the same API key as the REST API,
// as X-API-Key or a bearer token, and offers the REST API's reads as tools.
func (s *Server) setupMCPRoute() {
	deps := mcptransport.Deps{
		Kubernetes:     s.k8sClient,
		Topology:       s.resourceMapper,
		Troubleshooter: s.troubleshootCorrelator,
		Version:        mcptransport.Version,
	}
	// Assigned only when present: a nil *argocd.Client stored in the interface
	// would not compare equal to nil, and the ArgoCD tools would be offered
	// with nothing behind them.
	if s.argoClient != nil {
		deps.ArgoCD = s.argoClient
	}
	s.router.Handle("/mcp", s.authMiddleware(mcptransport.NewHTTPHandler(mcptransport.NewServer(&deps))))
}
