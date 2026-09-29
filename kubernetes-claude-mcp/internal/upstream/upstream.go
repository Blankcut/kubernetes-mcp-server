// Package upstream holds what the ArgoCD and GitLab API clients share about
// calling an upstream HTTP API: the error for a response outside 2xx, and the
// policy for which failures are worth retrying.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxErrorBody bounds how much of an error response is kept for diagnostics.
const maxErrorBody = 4096

// StatusError is returned for an HTTP response outside 2xx. Callers use
// StatusCode to tell a rejected token (401) from a denial (403) or a missing
// object (404) without parsing the message.
type StatusError struct {
	// Service names the upstream, e.g. "ArgoCD".
	Service    string
	StatusCode int
	// Body is the start of the response body.
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s API error (status %d): %s", e.Service, e.StatusCode, e.Body)
}

// NewStatusError reads the start of resp's body, closes it, and returns the
// StatusError describing resp.
func NewStatusError(service string, resp *http.Response) *StatusError {
	defer func() { _ = resp.Body.Close() }()

	statusErr := &StatusError{Service: service, StatusCode: resp.StatusCode}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil {
		statusErr.Body = fmt.Sprintf("failed to read response body: %v", err)
	} else {
		statusErr.Body = string(body)
	}
	return statusErr
}

// StatusCode returns the HTTP status of the StatusError in err's chain, or 0
// when err did not come from an HTTP response.
func StatusCode(err error) int {
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode
	}
	return 0
}

// TransportError reports a request that got no HTTP response at all:
// connection refused, TLS failure, timeout.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return "request failed: " + e.Err.Error() }

func (e *TransportError) Unwrap() error { return e.Err }

// Retryable reports whether a failed request is worth sending again.
//
// Only failures that can clear on their own qualify: a transport error, 429,
// or a 5xx. Any other status is the upstream's considered answer (a revoked
// token, a denial, a missing object), and repeating the request only delays
// it. Errors from building the request, such as missing credentials, are
// equally permanent.
//
// A transport error or 5xx may arrive after the upstream acted on the
// request, so those are retried only for idempotent methods: a retried POST
// could, for example, create the same merge request comment twice. A 429 is
// retried for every method, because a rate-limited request was not processed.
func Retryable(method string, err error) bool {
	var transportErr *TransportError
	switch code := StatusCode(err); {
	case code == http.StatusTooManyRequests:
		return true
	case code >= 500 && code <= 599:
		return idempotent(method)
	case code != 0:
		return false
	case errors.As(err, &transportErr):
		return idempotent(method)
	default:
		return false
	}
}

func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// Backoff says how many times a request is attempted and how long to wait
// between attempts.
type Backoff struct {
	// Attempts is the total number of attempts, including the first.
	Attempts int
	// Delay is the wait before the first retry; it doubles for each one after.
	Delay time.Duration
}

// DefaultBackoff makes up to three attempts, 1s and then 2s apart.
var DefaultBackoff = Backoff{Attempts: 3, Delay: time.Second}

// Do calls send until it succeeds, fails in a way Retryable rejects, or the
// attempts run out, and returns the last result. onRetry, if set, is told
// about each retry before the wait. Cancelling ctx during a wait returns
// ctx.Err() immediately.
func (b Backoff) Do(
	ctx context.Context,
	method string,
	send func() (*http.Response, error),
	onRetry func(attempt int, delay time.Duration, err error),
) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := send()
		if err == nil {
			return resp, nil
		}
		if attempt >= b.Attempts || !Retryable(method, err) {
			return nil, err
		}

		delay := b.Delay << (attempt - 1)
		if onRetry != nil {
			onRetry(attempt, delay, err)
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}
}
