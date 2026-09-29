package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/upstream"
)

// DefaultAuthzRefreshInterval is how often WatchApplicationsAccess re-checks
// that this server's token may read applications. A startup-only check is not
// enough: the RBAC grant was wiped on 2026-09-28 while the server was running.
const DefaultAuthzRefreshInterval = 60 * time.Second

// authzProbeTimeout bounds one probe, including doRequest's retries, so an
// unreachable ArgoCD cannot stall the refresh loop.
const authzProbeTimeout = 15 * time.Second

// applicationsGetProbeEndpoint asks ArgoCD whether this token may get every
// application in every project.
//
// The object is "*/*", not "*". ArgoCD globs each policy's object against the
// object in the question, and application policies are written as
// <project>/<app>: role:readonly grants "applications, get, */*". A bare "*"
// has no slash, never matches "*/*", and would report a correctly configured
// token as denied.
//
// The wildcards are deliberately left unescaped. attemptRequest assigns this to
// url.URL.Path, which escapes them once on the wire; pre-escaping them with
// url.PathEscape would send %252A, which ArgoCD reads as a literal "%2A" and
// again answers "no".
const applicationsGetProbeEndpoint = "/api/v1/account/can-i/applications/get/*/*"

// AuthzState is the outcome of the applications:get probe.
type AuthzState string

// Probe outcomes. Only denied and unauthenticated are definitive refusals;
// unknown and unavailable mean there is no answer to act on.
const (
	// AuthzUnknown means no probe has completed yet.
	AuthzUnknown AuthzState = "unknown"
	// AuthzAuthorized means ArgoCD answered "yes".
	AuthzAuthorized AuthzState = "authorized"
	// AuthzDenied means the token authenticates but ArgoCD answered "no".
	AuthzDenied AuthzState = "denied"
	// AuthzUnauthenticated means ArgoCD rejected the token itself (HTTP 401).
	AuthzUnauthenticated AuthzState = "unauthenticated"
	// AuthzUnavailable means ArgoCD could not be reached or did not return a
	// usable answer (transport error, unexpected status, malformed body).
	AuthzUnavailable AuthzState = "unavailable"
)

var (
	// ErrApplicationsGetDenied is the reason reported for AuthzDenied.
	ErrApplicationsGetDenied = errors.New("the ArgoCD token authenticates but lacks applications:get; check argocd-rbac-cm")
	// ErrTokenRejected is the reason reported for AuthzUnauthenticated.
	ErrTokenRejected = errors.New("ArgoCD rejected the token (HTTP 401); check ARGOCD_TOKEN and that its account is enabled")
)

// AuthzStatus is the result of one applications:get probe.
type AuthzStatus struct {
	State AuthzState
	// CheckedAt is when the probe ran; zero until the first probe completes.
	CheckedAt time.Time
	// Err explains any state other than authorized or unknown.
	Err error
}

// Reachable reports whether ArgoCD returned a usable answer.
func (s AuthzStatus) Reachable() bool {
	switch s.State {
	case AuthzAuthorized, AuthzDenied, AuthzUnauthenticated:
		return true
	default:
		return false
	}
}

// Refused reports whether ArgoCD definitively refused this token application
// reads, as opposed to the answer being unknown.
func (s AuthzStatus) Refused() bool {
	return s.State == AuthzDenied || s.State == AuthzUnauthenticated
}

// CheckApplicationsAccess asks ArgoCD, once, whether this server's token may
// get applications. It does not update the cached status; request paths should
// read ApplicationsAccess instead.
func (c *Client) CheckApplicationsAccess(ctx context.Context) AuthzStatus {
	status := AuthzStatus{CheckedAt: time.Now()}

	resp, err := c.doRequest(ctx, http.MethodGet, applicationsGetProbeEndpoint, nil)
	switch {
	case upstream.StatusCode(err) == http.StatusUnauthorized:
		// doRequest reports every non-2xx status as an error, but a 401 is
		// still ArgoCD's answer: it rejected the token itself.
		status.State, status.Err = AuthzUnauthenticated, ErrTokenRejected
		return status
	case err != nil:
		status.State, status.Err = AuthzUnavailable, err
		return status
	}
	defer func() { _ = resp.Body.Close() }()

	status.State, status.Err = parseCanIResponse(resp.StatusCode, resp.Body)
	return status
}

// parseCanIResponse interprets ArgoCD's can-i answer, {"value":"yes"|"no"}.
// Anything other than an exact yes or no is reported as unavailable rather
// than guessed at, because a false "denied" turns an empty list into an error.
func parseCanIResponse(statusCode int, body io.Reader) (AuthzState, error) {
	switch statusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return AuthzUnauthenticated, ErrTokenRejected
	default:
		return AuthzUnavailable, fmt.Errorf("unexpected status %d from ArgoCD can-i", statusCode)
	}

	var answer struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 4096)).Decode(&answer); err != nil {
		return AuthzUnavailable, fmt.Errorf("failed to decode ArgoCD can-i response: %w", err)
	}

	switch answer.Value {
	case "yes":
		return AuthzAuthorized, nil
	case "no":
		return AuthzDenied, ErrApplicationsGetDenied
	default:
		return AuthzUnavailable, fmt.Errorf("unexpected ArgoCD can-i answer %q", answer.Value)
	}
}

// ApplicationsAccess returns the cached result of the most recent probe. It
// never calls ArgoCD, so it is safe on hot paths such as readiness.
func (c *Client) ApplicationsAccess() AuthzStatus {
	c.authzMu.RLock()
	defer c.authzMu.RUnlock()
	return c.authz
}

// RefreshApplicationsAccess runs the probe, caches the result, and logs when
// the state changes, so a flip to denied is logged once at ERROR rather than on
// every interval.
func (c *Client) RefreshApplicationsAccess(ctx context.Context) AuthzStatus {
	probeCtx, cancel := context.WithTimeout(ctx, authzProbeTimeout)
	defer cancel()

	status := c.CheckApplicationsAccess(probeCtx)

	// A probe cut short by our own shutdown says nothing about ArgoCD.
	if ctx.Err() != nil {
		return status
	}

	c.authzMu.Lock()
	previous := c.authz.State
	c.authz = status
	c.authzMu.Unlock()

	if status.State != previous {
		c.logAuthzTransition(previous, status)
	}
	return status
}

// WatchApplicationsAccess probes immediately and then every interval until ctx
// is cancelled. Run it in its own goroutine.
func (c *Client) WatchApplicationsAccess(ctx context.Context, interval time.Duration) {
	c.RefreshApplicationsAccess(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.RefreshApplicationsAccess(ctx)
		}
	}
}

// logAuthzTransition logs a change in the probe's answer.
func (c *Client) logAuthzTransition(previous AuthzState, status AuthzStatus) {
	switch status.State {
	case AuthzAuthorized:
		c.logger.Infow("ArgoCD authorization confirmed", "check", "applications:get", "previous", previous)
	case AuthzDenied, AuthzUnauthenticated:
		// ERROR rather than WARN: this is the state that made the applications
		// list come back empty for ~16 hours on 2026-09-28 while every health
		// check stayed green.
		c.logger.Errorw("ArgoCD token cannot read applications",
			"state", status.State,
			"error", status.Err,
			"previous", previous)
	case AuthzUnavailable:
		c.logger.Warnw("ArgoCD authorization check could not complete",
			"error", status.Err,
			"previous", previous)
	}
}
