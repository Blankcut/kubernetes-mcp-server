package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// The MCP endpoint serves the same cluster reads as the REST API, so it takes
// the same key.
func TestMCPEndpointNeedsTheAPIKey(t *testing.T) {
	s := &Server{router: mux.NewRouter(), logger: logging.NewLogger()}
	s.config.Auth.APIKey = "the-key"
	s.setupMCPRoute()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	post := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, req)
		return rec
	}

	if rec := post(""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no key: status %d, want 401", rec.Code)
	}
	if rec := post("wrong"); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong key: status %d, want 401", rec.Code)
	}
	if rec := post("the-key"); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusNotFound {
		t.Errorf("right key: status %d, want the request to reach the MCP handler", rec.Code)
	}
}
