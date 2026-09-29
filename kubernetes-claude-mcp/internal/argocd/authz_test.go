package argocd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// newTestClient returns a token-authenticated client pointed at handler.
func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// Keep an ambient ARGOCD_TOKEN from overriding the config token.
	t.Setenv("ARGOCD_AUTH_TOKEN", "")
	t.Setenv("ARGOCD_TOKEN", "")

	cfg := &config.Config{
		ArgoCD: config.ArgoCDConfig{URL: server.URL, AuthToken: "test-token"},
		Claude: config.ClaudeConfig{APIKey: "test-claude-key"},
	}
	credProvider := auth.NewCredentialProvider(cfg)
	if err := credProvider.LoadCredentials(context.Background()); err != nil {
		t.Fatalf("Failed to load credentials: %v", err)
	}

	return NewClient(&cfg.ArgoCD, credProvider, logging.NewLogger())
}

// canIServer answers can-i with whatever answer currently holds, and counts
// the probes it receives.
type canIServer struct {
	answer atomic.Value // string: raw JSON body
	hits   atomic.Int32
}

func newCanIServer(answer string) *canIServer {
	s := &canIServer{}
	s.answer.Store(answer)
	return s
}

func (s *canIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.hits.Add(1)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(s.answer.Load().(string)))
}

func TestParseCanIResponse(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantState AuthzState
		wantErr   error // checked with errors.Is when set
	}{
		{name: "yes", status: http.StatusOK, body: `{"value":"yes"}`, wantState: AuthzAuthorized},
		{name: "no", status: http.StatusOK, body: `{"value":"no"}`, wantState: AuthzDenied, wantErr: ErrApplicationsGetDenied},
		{name: "unexpected value", status: http.StatusOK, body: `{"value":"maybe"}`, wantState: AuthzUnavailable},
		{name: "missing value", status: http.StatusOK, body: `{}`, wantState: AuthzUnavailable},
		{name: "malformed", status: http.StatusOK, body: `<html>502 Bad Gateway</html>`, wantState: AuthzUnavailable},
		{name: "empty body", status: http.StatusOK, body: ``, wantState: AuthzUnavailable},
		{name: "401", status: http.StatusUnauthorized, body: `{"error":"invalid session"}`, wantState: AuthzUnauthenticated, wantErr: ErrTokenRejected},
		{name: "non-200 with a yes body", status: http.StatusNoContent, body: `{"value":"yes"}`, wantState: AuthzUnavailable},
		{name: "500", status: http.StatusInternalServerError, body: `{"value":"no"}`, wantState: AuthzUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := parseCanIResponse(tt.status, strings.NewReader(tt.body))
			if state != tt.wantState {
				t.Errorf("state = %q, want %q", state, tt.wantState)
			}
			switch {
			case tt.wantState == AuthzAuthorized && err != nil:
				t.Errorf("authorized answer returned error: %v", err)
			case tt.wantState != AuthzAuthorized && err == nil:
				t.Errorf("state %q returned no error explaining it", state)
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// Argo CD matches the probe's object against each policy's glob, and
// role:readonly grants "applications, get, */*". Asking about a bare "*" never
// matches that and reports the fixed, working grant as denied. The wildcards
// must also reach Argo CD escaped exactly once: a double-escaped %252A is read
// as the literal "%2A", which is denied too.
func TestCheckApplicationsAccessAsksAboutEveryProjectAndApp(t *testing.T) {
	var gotPath, gotRequestURI, gotAuth string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRequestURI = r.RequestURI
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"value":"yes"}`))
	}))

	status := client.CheckApplicationsAccess(context.Background())

	if status.State != AuthzAuthorized {
		t.Fatalf("state = %q (err %v), want authorized", status.State, status.Err)
	}
	if want := "/api/v1/account/can-i/applications/get/*/*"; gotPath != want {
		t.Errorf("decoded path = %q, want %q", gotPath, want)
	}
	if strings.Contains(gotRequestURI, "%25") {
		t.Errorf("request URI %q is double-escaped", gotRequestURI)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want the configured token", gotAuth)
	}
	if status.CheckedAt.IsZero() {
		t.Error("CheckedAt was not set")
	}
}

func TestCheckApplicationsAccessStates(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    AuthzState
	}{
		{
			name: "denied",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"value":"no"}`))
			},
			want: AuthzDenied,
		},
		{
			name: "token rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid session: token is expired"}`))
			},
			want: AuthzUnauthenticated,
		},
		{
			// can-i says no with a 200 and {"value":"no"}. An error status is
			// no answer at all, so it must not read as a denial.
			name: "error status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			},
			want: AuthzUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.handler)

			// Neither 401 nor 403 is retried. The timeout keeps a regression
			// to retrying them from adding a 1s+2s backoff to the test.
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			if got := client.CheckApplicationsAccess(ctx); got.State != tt.want {
				t.Errorf("state = %q (err %v), want %q", got.State, got.Err, tt.want)
			}
		})
	}
}

func TestCheckApplicationsAccessUnreachable(t *testing.T) {
	client := newTestClient(t, http.NotFoundHandler())
	client.baseURL = "http://127.0.0.1:1" // nothing listens here

	// Bound doRequest's retry backoff.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	status := client.CheckApplicationsAccess(ctx)
	if status.State != AuthzUnavailable || status.Reachable() || status.Refused() {
		t.Errorf("status = %+v, want unavailable, unreachable and not refused", status)
	}
}

func TestApplicationsAccessIsCached(t *testing.T) {
	argo := newCanIServer(`{"value":"no"}`)
	client := newTestClient(t, argo)

	if got := client.ApplicationsAccess(); got.State != AuthzUnknown || got.Refused() || got.Reachable() {
		t.Fatalf("before any probe: %+v, want unknown", got)
	}

	client.RefreshApplicationsAccess(context.Background())
	for i := 0; i < 5; i++ {
		if got := client.ApplicationsAccess(); got.State != AuthzDenied || !got.Refused() {
			t.Fatalf("cached status = %+v, want denied", got)
		}
	}
	if hits := argo.hits.Load(); hits != 1 {
		t.Fatalf("reads went to ArgoCD: %d probes, want 1", hits)
	}

	// The grant is restored, but nothing is re-asked until the next refresh.
	argo.answer.Store(`{"value":"yes"}`)
	if got := client.ApplicationsAccess(); got.State != AuthzDenied {
		t.Fatalf("status changed without a refresh: %+v", got)
	}
	client.RefreshApplicationsAccess(context.Background())
	if got := client.ApplicationsAccess(); got.State != AuthzAuthorized {
		t.Fatalf("after refresh: %+v, want authorized", got)
	}
}

// A probe cut short by shutdown reflects our context, not ArgoCD, and must not
// overwrite the last real answer.
func TestRefreshApplicationsAccessIgnoresCancelledProbe(t *testing.T) {
	argo := newCanIServer(`{"value":"yes"}`)
	client := newTestClient(t, argo)
	client.RefreshApplicationsAccess(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.RefreshApplicationsAccess(ctx)

	if got := client.ApplicationsAccess(); got.State != AuthzAuthorized {
		t.Errorf("cancelled refresh overwrote the cache: %+v", got)
	}
}

// The 2026-09-28 wipe happened while the server was running, so the watch
// must notice a grant disappearing after startup. Readers run throughout so
// that -race covers the cache.
func TestWatchApplicationsAccessPicksUpRBACChanges(t *testing.T) {
	argo := newCanIServer(`{"value":"yes"}`)
	client := newTestClient(t, argo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		client.WatchApplicationsAccess(ctx, 10*time.Millisecond)
	}()

	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for ctx.Err() == nil {
				_ = client.ApplicationsAccess()
				time.Sleep(100 * time.Microsecond)
			}
		}()
	}

	waitForState(t, client, AuthzAuthorized)
	argo.answer.Store(`{"value":"no"}`)
	waitForState(t, client, AuthzDenied)

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WatchApplicationsAccess did not return after its context was cancelled")
	}
	readers.Wait()
}

func waitForState(t *testing.T, client *Client, want AuthzState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.ApplicationsAccess().State == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state never became %q; last %+v", want, client.ApplicationsAccess())
}
