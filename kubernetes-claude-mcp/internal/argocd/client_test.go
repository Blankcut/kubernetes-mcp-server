package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/auth"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/upstream"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// fastBackoff keeps retry tests quick without changing how many attempts
// are made.
var fastBackoff = upstream.Backoff{Attempts: 3, Delay: time.Millisecond}

// newSessionTestClient returns a username/password client pointed at handler,
// the only configuration whose token can be refreshed after a 401.
func newSessionTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	for _, key := range []string{"ARGOCD_AUTH_TOKEN", "ARGOCD_TOKEN", "ARGOCD_USERNAME", "ARGOCD_PASSWORD"} {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		ArgoCD: config.ArgoCDConfig{URL: server.URL, Username: "mcp", Password: "test-password"},
		Claude: config.ClaudeConfig{APIKey: "test-claude-key"},
	}
	credProvider := auth.NewCredentialProvider(cfg)
	if err := credProvider.LoadCredentials(context.Background()); err != nil {
		t.Fatalf("Failed to load credentials: %v", err)
	}

	client := NewClient(&cfg.ArgoCD, credProvider, logging.NewLogger())
	client.backoff = fastBackoff
	return client
}

// The bug: attemptRequest handed 401s back as successful responses, so a
// revoked token made ListApplications decode the error body into an empty
// list, and the refresh-on-401 branch in doRequest could never run.
func TestRejectedTokenIsAnErrorNotAnEmptyList(t *testing.T) {
	var hits atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid session: token is expired","code":16}`))
	}))
	client.backoff = fastBackoff

	apps, err := client.ListApplications(context.Background())

	if err == nil {
		t.Fatalf("ListApplications returned %d applications and no error for a 401", len(apps))
	}
	if code := upstream.StatusCode(err); code != http.StatusUnauthorized {
		t.Errorf("status code = %d (err %v), want 401", code, err)
	}
	// A configured API token has nothing to refresh from, and a 401 is not
	// transient, so there is exactly one request.
	if n := hits.Load(); n != 1 {
		t.Errorf("ArgoCD received %d requests, want 1", n)
	}
}

// sessionServer mints session tokens "session-1", "session-2", ... and accepts
// only the tokens in valid on /api/v1/applications/echo/sync.
type sessionServer struct {
	valid    func(token string) bool
	sessions atomic.Int32
	syncs    atomic.Int32
	// prunes records the prune flag of each sync request that got through.
	prunes chan bool
}

func (s *sessionServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/session":
		n := s.sessions.Add(1)
		_, _ = fmt.Fprintf(w, `{"token":"session-%d"}`, n)
	case "/api/v1/applications/echo/sync":
		s.syncs.Add(1)
		if !s.valid(r.Header.Get("Authorization")) {
			http.Error(w, `{"error":"invalid session: token is expired"}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			Prune bool `json:"prune"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		s.prunes <- body.Prune
		_, _ = w.Write([]byte(`{}`))
	default:
		http.NotFound(w, r)
	}
}

func TestSessionIsRefreshedOnceAfterA401(t *testing.T) {
	t.Run("retry succeeds with the new session", func(t *testing.T) {
		// session-1 is what addAuth mints on the first request; it has
		// "expired" by the time ArgoCD sees it.
		argo := &sessionServer{
			valid:  func(token string) bool { return token == "Bearer session-2" },
			prunes: make(chan bool, 4),
		}
		client := newSessionTestClient(t, argo)

		// A POST, so the retry also proves the body is re-sent in full.
		if err := client.SyncApplication(context.Background(), "echo", "main", true, nil); err != nil {
			t.Fatalf("SyncApplication: %v", err)
		}

		if n := argo.syncs.Load(); n != 2 {
			t.Errorf("sync requests = %d, want 2 (the 401, then one retry)", n)
		}
		if n := argo.sessions.Load(); n != 2 {
			t.Errorf("sessions created = %d, want 2 (initial, then one refresh)", n)
		}
		// The handler ran before SyncApplication returned, so a request that
		// got through has already been recorded.
		select {
		case prune := <-argo.prunes:
			if !prune {
				t.Error("the retried request lost its body: prune arrived false")
			}
		default:
			t.Error("no sync request was accepted by ArgoCD")
		}
		creds, err := client.credentialProvider.GetCredentials(auth.ServiceArgoCD)
		if err != nil || creds.Token != "session-2" {
			t.Errorf("stored token = %q (err %v), want the refreshed session-2", creds.Token, err)
		}
	})

	t.Run("still rejected after the refresh", func(t *testing.T) {
		argo := &sessionServer{
			valid:  func(string) bool { return false },
			prunes: make(chan bool, 4),
		}
		client := newSessionTestClient(t, argo)

		err := client.SyncApplication(context.Background(), "echo", "main", true, nil)

		if code := upstream.StatusCode(err); code != http.StatusUnauthorized {
			t.Fatalf("err = %v, want a 401 StatusError", err)
		}
		// One refresh and one retry, not a loop.
		if n := argo.syncs.Load(); n != 2 {
			t.Errorf("sync requests = %d, want 2", n)
		}
		if n := argo.sessions.Load(); n != 2 {
			t.Errorf("sessions created = %d, want 2", n)
		}
	})
}

// countingServer answers every request with the statuses in order, then 200.
func countingServer(statuses ...int) (http.Handler, *atomic.Int32) {
	var hits atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(hits.Add(1))
		if n <= len(statuses) {
			http.Error(w, http.StatusText(statuses[n-1]), statuses[n-1])
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"echo"}}]}`))
	}), &hits
}

// 403 and 404 used to be retried twice with a 1s+2s backoff, adding three
// seconds to an answer that could never change.
func TestPermanentStatusesAreNotRetried(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			handler, hits := countingServer(code, code, code)
			client := newTestClient(t, handler)
			client.backoff = fastBackoff

			_, err := client.ListApplications(context.Background())

			if got := upstream.StatusCode(err); got != code {
				t.Errorf("err = %v, want status %d", err, code)
			}
			if n := hits.Load(); n != 1 {
				t.Errorf("ArgoCD received %d requests, want 1", n)
			}
		})
	}
}

func TestTransientStatusesAreRetried(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			handler, hits := countingServer(code)
			client := newTestClient(t, handler)
			client.backoff = fastBackoff

			apps, err := client.ListApplications(context.Background())

			if err != nil || len(apps) != 1 {
				t.Fatalf("ListApplications = %d apps, %v; want 1 app after a retry", len(apps), err)
			}
			if n := hits.Load(); n != 2 {
				t.Errorf("ArgoCD received %d requests, want 2", n)
			}
		})
	}
}

// The production backoff is 1s then 2s. A caller that gives up must not be
// held for it.
func TestRetryBackoffStopsWhenContextEnds(t *testing.T) {
	handler, hits := countingServer(http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusServiceUnavailable)
	client := newTestClient(t, handler) // upstream.DefaultBackoff

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.ListApplications(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Errorf("ListApplications returned after %v; it sat out the backoff", elapsed)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("ArgoCD received %d requests, want 1", n)
	}
}
