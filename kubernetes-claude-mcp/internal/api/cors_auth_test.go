package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
	"github.com/gorilla/mux"
)

// routedServer has the real routes and middleware but no upstream clients, so
// it can answer only requests that never reach a handler: preflights and
// authentication failures.
func routedServer(apiKey string) *Server {
	s := &Server{router: mux.NewRouter(), logger: logging.NewLogger()}
	s.config.Auth.APIKey = apiKey
	s.setupRoutes()
	s.setupNamespaceRoutes()
	return s
}

func serve(s *Server, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// mux applies router middleware only to matched routes, and every route is
// GET- or POST-only, so preflights used to get a 404 or 405 without CORS
// headers and no browser could call the API.
func TestPreflightIsAnswered(t *testing.T) {
	s := routedServer("k")
	for _, path := range []string{
		"/api/v1/health",
		"/api/v1/namespaces",
		"/api/v1/mcp/resource",
		"/api/v1/namespaces/default/analysis",
	} {
		req := httptest.NewRequest(http.MethodOptions, path, http.NoBody)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "x-api-key, content-type")
		rec := serve(s, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("OPTIONS %s = %d, want 204", path, rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("OPTIONS %s: Access-Control-Allow-Origin = %q", path, got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-API-Key" {
			t.Errorf("OPTIONS %s: Access-Control-Allow-Headers = %q, must allow X-API-Key", path, got)
		}
	}
}

// Before this, the check was apiKey != configured, so with no key configured
// an empty presented key matched.
func TestNoConfiguredKeyRejectsEverything(t *testing.T) {
	s := routedServer("")
	for _, h := range [][2]string{
		{"X-API-Key", ""},
		{"Authorization", "Bearer "},
		{"X-API-Key", "anything"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces", http.NoBody)
		req.Header[h[0]] = []string{h[1]} // set verbatim, trailing space included
		if rec := serve(s, req); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %q with no configured key = %d, want 401", h[0], h[1], rec.Code)
		}
	}
}

func TestWrongKeyRejectedAndKeyCheckedOnNamespaceRoutes(t *testing.T) {
	s := routedServer("right")
	for _, path := range []string{"/api/v1/namespaces", "/api/v1/namespaces/default/analysis"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		req.Header.Set("X-API-Key", "wrong")
		if rec := serve(s, req); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with a wrong key = %d, want 401", path, rec.Code)
		}
		req = httptest.NewRequest(http.MethodGet, path, http.NoBody)
		req.Header.Set("Authorization", "Bearer wrong")
		if rec := serve(s, req); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with a wrong bearer = %d, want 401", path, rec.Code)
		}
	}
}
