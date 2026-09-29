package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/argocd"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/gitlab"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
	"github.com/gorilla/mux"
)

// fakeUpstream stands in for the Kubernetes, ArgoCD and GitLab APIs at once.
type fakeUpstream struct {
	k8sDown bool
	canI    http.HandlerFunc // /api/v1/account/can-i/...
	list    http.HandlerFunc // /api/v1/applications
	get     http.HandlerFunc // /api/v1/applications/{name}
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/version": // Kubernetes discovery
		if f.k8sDown {
			http.Error(w, "etcd unavailable", http.StatusInternalServerError)
			return
		}
		reply(http.StatusOK, `{"major":"1","minor":"33","gitVersion":"v1.33.0"}`)(w, r)
	case r.URL.Path == "/api/version": // ArgoCD
		reply(http.StatusOK, `{"version":"v3.4.3"}`)(w, r)
	case r.URL.Path == "/api/v4/version": // GitLab
		reply(http.StatusOK, `{"version":"17.0.0"}`)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/account/can-i/") && f.canI != nil:
		f.canI(w, r)
	case r.URL.Path == "/api/v1/applications" && f.list != nil:
		f.list(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/applications/") && f.get != nil:
		f.get(w, r)
	default:
		http.NotFound(w, r)
	}
}

func reply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

var (
	canIYes       = reply(http.StatusOK, `{"value":"yes"}`)
	canINo        = reply(http.StatusOK, `{"value":"no"}`)
	canIGarbled   = reply(http.StatusOK, `{"value":"maybe"}`)
	tokenRejected = reply(http.StatusUnauthorized, `{"error":"invalid session: token has invalid claims","code":16}`)
	emptyList     = reply(http.StatusOK, `{"items":[]}`)
	oneAppList    = reply(http.StatusOK, `{"items":[{"metadata":{"name":"echo","namespace":"argocd"}}]}`)
)

// newTestServer wires real clients to upstream. When probe is true the ArgoCD
// authorization cache is primed the way the watch loop would prime it.
func newTestServer(t *testing.T, upstream *fakeUpstream, probe bool) *Server {
	t.Helper()

	backend := httptest.NewServer(upstream)
	t.Cleanup(backend.Close)

	for _, key := range []string{"ARGOCD_AUTH_TOKEN", "ARGOCD_TOKEN", "GITLAB_AUTH_TOKEN", "GITLAB_TOKEN"} {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		ArgoCD: config.ArgoCDConfig{URL: backend.URL, AuthToken: "test-token"},
		GitLab: config.GitLabConfig{URL: backend.URL, AuthToken: "test-token", APIVersion: "v4"},
		Claude: config.ClaudeConfig{APIKey: "test-claude-key"},
	}
	credProvider := auth.NewCredentialProvider(cfg)
	if err := credProvider.LoadCredentials(context.Background()); err != nil {
		t.Fatalf("Failed to load credentials: %v", err)
	}

	logger := logging.NewLogger()
	k8sClient, err := k8s.NewClient(config.KubernetesConfig{KubeConfig: writeKubeconfig(t, backend.URL)}, logger)
	if err != nil {
		t.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	argoClient := argocd.NewClient(&cfg.ArgoCD, credProvider, logger)
	if probe {
		argoClient.RefreshApplicationsAccess(context.Background())
	}

	return &Server{
		k8sClient:    k8sClient,
		argoClient:   argoClient,
		gitlabClient: gitlab.NewClient(&cfg.GitLab, credProvider, logger),
		logger:       logger,
	}
}

func writeKubeconfig(t *testing.T, server string) string {
	t.Helper()
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- name: fake
  cluster:
    server: ` + server + `
contexts:
- name: fake
  context:
    cluster: fake
    user: fake
current-context: fake
users:
- name: fake
  user:
    token: fake
`
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}
	return path
}

func TestListArgoApplicationsWhenArgoCDRefusesTheToken(t *testing.T) {
	tests := []struct {
		name        string
		canI        http.HandlerFunc
		list        http.HandlerFunc
		probe       bool
		wantCode    int
		wantApps    int
		wantDetails string // substring of the error details, for error responses
	}{
		{
			// The 2026-09-28 incident: RBAC gone, ArgoCD filters every app out.
			name:        "empty and denied",
			canI:        canINo,
			list:        emptyList,
			probe:       true,
			wantCode:    http.StatusServiceUnavailable,
			wantDetails: "argocd-rbac-cm",
		},
		{
			// A revoked token: the 401 body decodes to an empty list as well.
			name:        "empty and token rejected",
			canI:        tokenRejected,
			list:        tokenRejected,
			probe:       true,
			wantCode:    http.StatusServiceUnavailable,
			wantDetails: "401",
		},
		{
			// The client now reports a 401 as an error instead of decoding it
			// into an empty list, so it needs no probe result to be caught.
			name:        "token rejected before the first probe",
			canI:        tokenRejected,
			list:        tokenRejected,
			probe:       false,
			wantCode:    http.StatusServiceUnavailable,
			wantDetails: "401",
		},
		{
			name:     "empty and authorized",
			canI:     canIYes,
			list:     emptyList,
			probe:    true,
			wantCode: http.StatusOK,
		},
		{
			name:     "empty and probe unavailable",
			canI:     canIGarbled,
			list:     emptyList,
			probe:    true,
			wantCode: http.StatusOK,
		},
		{
			name:     "empty and not probed yet",
			canI:     canINo,
			list:     emptyList,
			probe:    false,
			wantCode: http.StatusOK,
		},
		{
			// A project-scoped grant answers no for */* but still sees its apps.
			name:     "non-empty and denied",
			canI:     canINo,
			list:     oneAppList,
			probe:    true,
			wantCode: http.StatusOK,
			wantApps: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, &fakeUpstream{canI: tt.canI, list: tt.list}, tt.probe)

			rec := httptest.NewRecorder()
			s.handleListArgoApplications(rec, httptest.NewRequest(http.MethodGet, "/api/v1/argocd/applications", http.NoBody))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantCode, rec.Body)
			}

			if tt.wantCode != http.StatusOK {
				var body struct {
					Error   string `json:"error"`
					Details string `json:"details"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if body.Error != "ArgoCD denied applications:get for this server's token" {
					t.Errorf("error = %q", body.Error)
				}
				if !strings.Contains(body.Details, tt.wantDetails) {
					t.Errorf("details = %q, want it to mention %q", body.Details, tt.wantDetails)
				}
				return
			}

			var body struct {
				Applications []json.RawMessage `json:"applications"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode list body: %v", err)
			}
			if len(body.Applications) != tt.wantApps {
				t.Errorf("got %d applications, want %d", len(body.Applications), tt.wantApps)
			}
		})
	}
}

// A rejected token used to come back from GET /argocd/applications/{name} as
// 200 and an application with every field empty.
func TestGetArgoApplicationErrors(t *testing.T) {
	tests := []struct {
		name        string
		get         http.HandlerFunc
		wantCode    int
		wantError   string
		wantDetails string
	}{
		{
			name:        "token rejected",
			get:         tokenRejected,
			wantCode:    http.StatusServiceUnavailable,
			wantError:   "ArgoCD denied applications:get for this server's token",
			wantDetails: "401",
		},
		{
			name:        "not found",
			get:         reply(http.StatusNotFound, `{"error":"applications.argoproj.io \"echo\" not found","code":5}`),
			wantCode:    http.StatusInternalServerError,
			wantError:   "Failed to get ArgoCD application",
			wantDetails: "status 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, &fakeUpstream{get: tt.get}, false)

			rec := httptest.NewRecorder()
			req := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/v1/argocd/applications/echo", http.NoBody),
				map[string]string{"name": "echo"})
			s.handleGetArgoApplication(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantCode, rec.Body)
			}
			var body struct {
				Error   string `json:"error"`
				Details string `json:"details"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body.Error != tt.wantError {
				t.Errorf("error = %q, want %q", body.Error, tt.wantError)
			}
			if !strings.Contains(body.Details, tt.wantDetails) {
				t.Errorf("details = %q, want it to mention %q", body.Details, tt.wantDetails)
			}
		})
	}
}

type argoCDComponent struct {
	Status     string `json:"status"`
	Reachable  bool   `json:"reachable"`
	Authorized *bool  `json:"authorized"`
	Message    string `json:"message"`
}

// Readiness gates on Kubernetes only. A denied ArgoCD must show up in the
// response without failing the probe: a failing probe would pull the pod out
// of service and take the non-ArgoCD tools down with it.
func TestReadinessReportsArgoCDWithoutGatingOnIt(t *testing.T) {
	tests := []struct {
		name           string
		upstream       fakeUpstream
		probe          bool
		wantCode       int
		wantStatus     string
		wantArgoStatus string
		wantReachable  bool
		wantAuthorized *bool
	}{
		{
			name:           "denied",
			upstream:       fakeUpstream{canI: canINo},
			probe:          true,
			wantCode:       http.StatusOK,
			wantStatus:     "degraded",
			wantArgoStatus: "denied",
			wantReachable:  true,
			wantAuthorized: boolPtr(false),
		},
		{
			name:           "token rejected",
			upstream:       fakeUpstream{canI: tokenRejected},
			probe:          true,
			wantCode:       http.StatusOK,
			wantStatus:     "degraded",
			wantArgoStatus: "unauthenticated",
			wantReachable:  true,
			wantAuthorized: boolPtr(false),
		},
		{
			name:           "authorized",
			upstream:       fakeUpstream{canI: canIYes},
			probe:          true,
			wantCode:       http.StatusOK,
			wantStatus:     "ready",
			wantArgoStatus: "authorized",
			wantReachable:  true,
			wantAuthorized: boolPtr(true),
		},
		{
			name:           "unavailable",
			upstream:       fakeUpstream{canI: canIGarbled},
			probe:          true,
			wantCode:       http.StatusOK,
			wantStatus:     "ready",
			wantArgoStatus: "unavailable",
		},
		{
			name:           "not probed yet",
			upstream:       fakeUpstream{canI: canINo},
			probe:          false,
			wantCode:       http.StatusOK,
			wantStatus:     "ready",
			wantArgoStatus: "unknown",
		},
		{
			// Kubernetes still decides readiness exactly as before.
			name:           "kubernetes down",
			upstream:       fakeUpstream{canI: canIYes, k8sDown: true},
			probe:          true,
			wantCode:       http.StatusServiceUnavailable,
			wantStatus:     "not ready",
			wantArgoStatus: "authorized",
			wantReachable:  true,
			wantAuthorized: boolPtr(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t, &tt.upstream, tt.probe)

			rec := httptest.NewRecorder()
			s.handleReadiness(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", http.NoBody))

			if rec.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d; body %s", rec.Code, tt.wantCode, rec.Body)
			}

			var body struct {
				Status     string          `json:"status"`
				Ready      bool            `json:"ready"`
				Checks     map[string]bool `json:"checks"`
				Components struct {
					ArgoCD argoCDComponent `json:"argocd"`
				} `json:"components"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode readiness body: %v", err)
			}

			if body.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", body.Status, tt.wantStatus)
			}
			if body.Ready != (tt.wantCode == http.StatusOK) {
				t.Errorf("ready = %v with HTTP %d", body.Ready, rec.Code)
			}
			if _, gated := body.Checks["argocd"]; gated {
				t.Error("argocd appears under checks; it must not gate readiness")
			}
			assertArgoCDComponent(t, body.Components.ArgoCD, tt.wantArgoStatus, tt.wantReachable, tt.wantAuthorized)
		})
	}
}

// /health used to report ArgoCD "available" off connectivity alone, which is
// what it said throughout the 2026-09-28 wipe.
func TestHealthReportsArgoCDUnauthorized(t *testing.T) {
	s := newTestServer(t, &fakeUpstream{canI: canINo}, true)

	rec := httptest.NewRecorder()
	s.handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}

	var body struct {
		Status     string            `json:"status"`
		Services   map[string]string `json:"services"`
		Components struct {
			ArgoCD argoCDComponent `json:"argocd"`
		} `json:"components"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode health body: %v", err)
	}

	if body.Status != "degraded" {
		t.Errorf("status = %q, want degraded", body.Status)
	}
	if got := body.Services["argocd"]; got != "unauthorized" {
		t.Errorf("services.argocd = %q, want unauthorized", got)
	}
	assertArgoCDComponent(t, body.Components.ArgoCD, "denied", true, boolPtr(false))
}

func assertArgoCDComponent(t *testing.T, got argoCDComponent, wantStatus string, wantReachable bool, wantAuthorized *bool) {
	t.Helper()

	if got.Status != wantStatus {
		t.Errorf("components.argocd.status = %q, want %q", got.Status, wantStatus)
	}
	if got.Reachable != wantReachable {
		t.Errorf("components.argocd.reachable = %v, want %v", got.Reachable, wantReachable)
	}
	if formatBoolPtr(got.Authorized) != formatBoolPtr(wantAuthorized) {
		t.Errorf("components.argocd.authorized = %s, want %s",
			formatBoolPtr(got.Authorized), formatBoolPtr(wantAuthorized))
	}

	// Only refusals carry a message, and it is the fixed remediation text.
	refused := wantStatus == "denied" || wantStatus == "unauthenticated"
	if refused && got.Message == "" {
		t.Error("components.argocd.message is empty for a refusal")
	}
	if !refused && got.Message != "" {
		t.Errorf("components.argocd.message = %q, want none", got.Message)
	}
}

func boolPtr(b bool) *bool { return &b }

// formatBoolPtr renders nil as "null", matching the JSON for "no answer".
func formatBoolPtr(b *bool) string {
	if b == nil {
		return "null"
	}
	return strconv.FormatBool(*b)
}
