package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func statusErr(code int) error {
	return &StatusError{Service: "ArgoCD", StatusCode: code}
}

func TestNewStatusError(t *testing.T) {
	body := &closeRecorder{Reader: strings.NewReader(`{"error":"permission denied"}`)}
	err := NewStatusError("ArgoCD", &http.Response{StatusCode: http.StatusForbidden, Body: body})

	if got, want := err.Error(), `ArgoCD API error (status 403): {"error":"permission denied"}`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !body.closed {
		t.Error("response body was not closed")
	}

	huge := io.NopCloser(strings.NewReader(strings.Repeat("x", 10*maxErrorBody)))
	if got := NewStatusError("GitLab", &http.Response{StatusCode: http.StatusBadGateway, Body: huge}); len(got.Body) != maxErrorBody {
		t.Errorf("kept %d bytes of the body, want %d", len(got.Body), maxErrorBody)
	}
}

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

func TestStatusCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "status error", err: statusErr(http.StatusUnauthorized), want: http.StatusUnauthorized},
		{name: "wrapped", err: fmt.Errorf("failed to list ArgoCD applications: %w", statusErr(http.StatusNotFound)), want: http.StatusNotFound},
		{name: "transport error", err: &TransportError{Err: errors.New("connection refused")}, want: 0},
		{name: "plain error", err: errors.New("no valid ArgoCD credentials available"), want: 0},
		{name: "nil", err: nil, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusCode(tt.err); got != tt.want {
				t.Errorf("StatusCode = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRetryable(t *testing.T) {
	transport := &TransportError{Err: errors.New("dial tcp: connection refused")}

	tests := []struct {
		name   string
		method string
		err    error
		want   bool
	}{
		{"GET transport error", http.MethodGet, transport, true},
		{"GET wrapped transport error", http.MethodGet, fmt.Errorf("context: %w", transport), true},
		{"GET 429", http.MethodGet, statusErr(http.StatusTooManyRequests), true},
		{"GET 500", http.MethodGet, statusErr(http.StatusInternalServerError), true},
		{"GET 502", http.MethodGet, statusErr(http.StatusBadGateway), true},
		{"GET 503", http.MethodGet, statusErr(http.StatusServiceUnavailable), true},
		{"GET 504", http.MethodGet, statusErr(http.StatusGatewayTimeout), true},
		{"GET 400", http.MethodGet, statusErr(http.StatusBadRequest), false},
		{"GET 401", http.MethodGet, statusErr(http.StatusUnauthorized), false},
		{"GET 403", http.MethodGet, statusErr(http.StatusForbidden), false},
		{"GET 404", http.MethodGet, statusErr(http.StatusNotFound), false},
		{"GET 409", http.MethodGet, statusErr(http.StatusConflict), false},
		{"GET 3xx", http.MethodGet, statusErr(http.StatusNotModified), false},
		{"GET missing credentials", http.MethodGet, errors.New("no valid ArgoCD credentials available"), false},
		// The upstream may have acted on a POST before failing.
		{"POST transport error", http.MethodPost, transport, false},
		{"POST 503", http.MethodPost, statusErr(http.StatusServiceUnavailable), false},
		// A rate-limited request was not processed, so any method may retry.
		{"POST 429", http.MethodPost, statusErr(http.StatusTooManyRequests), true},
		{"POST 404", http.MethodPost, statusErr(http.StatusNotFound), false},
		{"PUT 503", http.MethodPut, statusErr(http.StatusServiceUnavailable), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Retryable(tt.method, tt.err); got != tt.want {
				t.Errorf("Retryable(%s, %v) = %v, want %v", tt.method, tt.err, got, tt.want)
			}
		})
	}
}

// sequence returns a send func that fails with errs in order, then succeeds,
// and a pointer to the number of calls made.
func sequence(errs ...error) (send func() (*http.Response, error), calls *int) {
	calls = new(int)
	return func() (*http.Response, error) {
		*calls++
		if *calls <= len(errs) {
			return nil, errs[*calls-1]
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	}, calls
}

func TestBackoffDo(t *testing.T) {
	backoff := Backoff{Attempts: 3, Delay: time.Millisecond}

	t.Run("retries until success", func(t *testing.T) {
		send, calls := sequence(statusErr(http.StatusServiceUnavailable), statusErr(http.StatusBadGateway))
		var delays []time.Duration
		resp, err := backoff.Do(context.Background(), http.MethodGet, send, func(_ int, delay time.Duration, _ error) {
			delays = append(delays, delay)
		})
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("Do = %v, %v; want success", resp, err)
		}
		if *calls != 3 {
			t.Errorf("sent %d times, want 3", *calls)
		}
		if len(delays) != 2 || delays[0] != time.Millisecond || delays[1] != 2*time.Millisecond {
			t.Errorf("delays = %v, want [1ms 2ms]", delays)
		}
	})

	t.Run("gives up after the last attempt", func(t *testing.T) {
		unavailable := statusErr(http.StatusServiceUnavailable)
		send, calls := sequence(unavailable, unavailable, unavailable, unavailable)
		_, err := backoff.Do(context.Background(), http.MethodGet, send, nil)
		if !errors.Is(err, unavailable) {
			t.Errorf("err = %v, want the last StatusError", err)
		}
		if *calls != 3 {
			t.Errorf("sent %d times, want 3", *calls)
		}
	})

	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(fmt.Sprintf("does not retry %d", code), func(t *testing.T) {
			send, calls := sequence(statusErr(code))
			_, err := backoff.Do(context.Background(), http.MethodGet, send, func(int, time.Duration, error) {
				t.Error("onRetry called for a permanent failure")
			})
			if StatusCode(err) != code {
				t.Errorf("err = %v, want status %d", err, code)
			}
			if *calls != 1 {
				t.Errorf("sent %d times, want 1", *calls)
			}
		})
	}
}

// A backoff wait must end as soon as the caller gives up; before this the
// probe and API handlers sat out a 1s+2s wait they no longer wanted.
func TestBackoffDoStopsWaitingWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	send, calls := sequence(statusErr(http.StatusServiceUnavailable), statusErr(http.StatusServiceUnavailable))
	start := time.Now()
	_, err := Backoff{Attempts: 3, Delay: time.Hour}.Do(ctx, http.MethodGet, send, func(int, time.Duration, error) {
		cancel()
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Do returned after %v; the wait ignored cancellation", elapsed)
	}
	if *calls != 1 {
		t.Errorf("sent %d times, want 1", *calls)
	}
}
