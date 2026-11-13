package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Logger is the part of the server's logger this package uses.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
}

const (
	// queueSize bounds the spans waiting to be sent. Past it, new spans are
	// dropped: this is reporting, and it must not grow without limit while an
	// endpoint is down.
	queueSize = 512
	// maxBatch and flushAfter decide when a batch goes out: when it is full,
	// or this long after its first span, whichever comes first.
	maxBatch   = 64
	flushAfter = 2 * time.Second
	// retryWait is the pause before the one retry of a failed export.
	retryWait = time.Second
	// warnEvery is the most often a failing endpoint is logged.
	warnEvery = time.Minute
)

// Reporter sends model calls to an OTLP/HTTP endpoint. A nil *Reporter is
// valid and reports nothing, so callers need not check whether reporting is
// on.
type Reporter struct {
	cfg     *Config
	version string
	logger  Logger
	client  *http.Client

	queue    chan span
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	// overflow counts spans dropped because the queue was full, until the
	// sending goroutine reports them.
	overflow atomic.Int64

	// Owned by the sending goroutine.
	lastWarned time.Time
	unreported int

	flushAfter time.Duration
	retryWait  time.Duration
	warnEvery  time.Duration
}

// FromEnv builds a Reporter from the standard OpenTelemetry environment
// variables (see ConfigFromEnv). It returns nil when reporting is not
// configured, or is configured in a way that cannot work; the second case is
// logged once. version is reported as service.version.
func FromEnv(logger Logger, version string) *Reporter {
	cfg, err := ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Warn("Model call reporting is off", "reason", err.Error())
		return nil
	}
	if cfg == nil {
		return nil
	}
	cfg.DetectKubernetes(os.Getenv)

	names := cfg.headerNames()
	sort.Strings(names)
	logger.Info("Reporting model calls as OpenTelemetry spans",
		"endpoint", cfg.displayEndpoint(),
		"headers", names,
		"service", cfg.Resource["service.name"])

	return New(cfg, logger, version)
}

// New starts a Reporter that sends to cfg.Endpoint. Call Shutdown before the
// process exits to send what is still queued.
func New(cfg *Config, logger Logger, version string) *Reporter {
	return newReporter(cfg, logger, version, flushAfter, retryWait, warnEvery)
}

func newReporter(cfg *Config, logger Logger, version string, flush, retry, warn time.Duration) *Reporter {
	r := &Reporter{
		cfg:     cfg,
		version: version,
		logger:  logger,
		client: &http.Client{
			Timeout: cfg.Timeout,
			// A redirect is not followed: it could carry the configured
			// headers to a host the operator never named.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		queue:      make(chan span, queueSize),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		flushAfter: flush,
		retryWait:  retry,
		warnEvery:  warn,
	}
	go r.run()
	return r
}

// ReportModelCall queues one model call for sending. It never blocks, never
// panics and returns nothing: a call that cannot be queued is dropped.
func (r *Reporter) ReportModelCall(call *ModelCall) {
	if r == nil || call == nil {
		return
	}
	defer func() { _ = recover() }()

	select {
	case <-r.stop:
		return
	default:
	}

	select {
	case r.queue <- buildSpan(call):
	default:
		r.overflow.Add(1)
	}
}

// Shutdown sends what is queued and stops the Reporter. It waits until that
// is done or ctx ends, whichever is first. Calls reported afterwards are
// dropped.
func (r *Reporter) Shutdown(ctx context.Context) {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.stop) })
	select {
	case <-r.done:
	case <-ctx.Done():
	}
}

// run is the sending goroutine: it gathers spans into batches and sends them.
func (r *Reporter) run() {
	defer close(r.done)

	var batch []span
	// Armed when a batch gets its first span.
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	flush := func() {
		if len(batch) > 0 {
			r.send(batch)
			batch = nil
		}
		if n := r.overflow.Swap(0); n > 0 {
			r.warn(int(n), "Model call spans were dropped: the queue was full")
		}
	}

	for {
		select {
		case s := <-r.queue:
			batch = append(batch, s)
			switch {
			case len(batch) >= maxBatch:
				timer.Stop()
				flush()
			case len(batch) == 1:
				timer.Reset(r.flushAfter)
			}
		case <-timer.C:
			flush()
		case <-r.stop:
			for {
				select {
				case s := <-r.queue:
					batch = append(batch, s)
					continue
				default:
				}
				break
			}
			flush()
			return
		}
	}
}

// send posts one batch. It tries twice, because a receiver may be restarting
// and a retried OTLP export carries the same span IDs; after that the batch is
// dropped. The model calls it describes returned long ago.
func (r *Reporter) send(spans []span) {
	attrs := make([]keyValue, 0, len(r.cfg.Resource)+1)
	keys := make([]string, 0, len(r.cfg.Resource))
	for key := range r.cfg.Resource {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		attrs = append(attrs, stringAttr(key, r.cfg.Resource[key]))
	}
	if r.version != "" && r.cfg.Resource["service.version"] == "" {
		attrs = append(attrs, stringAttr("service.version", r.version))
	}

	body, err := json.Marshal(exportRequest{ResourceSpans: []resourceSpans{{
		Resource:   resource{Attributes: attrs},
		ScopeSpans: []scopeSpans{{Scope: scope{Name: DefaultServiceName, Version: r.version}, Spans: spans}},
	}}})
	if err != nil {
		r.warn(len(spans), "Model call spans were dropped: they could not be encoded")
		return
	}

	var reason string
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(r.retryWait):
			case <-r.stop:
				// Shutting down: retry at once rather than hold up the exit.
			}
		}
		status, postErr := r.post(body)
		switch {
		case postErr != nil:
			reason = describe(postErr)
		case status >= 200 && status < 300:
			return
		case status == http.StatusTooManyRequests || status >= 500:
			reason = "HTTP " + strconv.Itoa(status)
		default:
			// The receiver understood and said no (a bad token, a wrong
			// path). Sending the same bytes again will not change that.
			r.warn(len(spans), "Model call spans were refused by the OTLP endpoint", "status", status)
			return
		}
	}
	r.warn(len(spans), "Model call spans could not be delivered to the OTLP endpoint", "reason", reason)
}

func (r *Reporter) post(body []byte) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, r.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	for name, value := range r.cfg.Headers {
		req.Header.Set(name, value)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// Read (a bounded amount of) the body so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}

// warn logs that count spans were lost, at most once every warnEvery. Spans
// lost in between are added to the next warning's count. Nothing from the
// request (its headers, its URL) is ever part of the message.
func (r *Reporter) warn(count int, msg string, keysAndValues ...interface{}) {
	r.unreported += count
	now := time.Now()
	if !r.lastWarned.IsZero() && now.Sub(r.lastWarned) < r.warnEvery {
		return
	}
	r.lastWarned = now
	fields := append([]interface{}{"spans", r.unreported, "endpoint", r.cfg.displayEndpoint()}, keysAndValues...)
	r.unreported = 0
	r.logger.Warn(msg, fields...)
}

// describe says why a request failed without repeating its URL, which
// net/http puts in the error text.
func describe(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return "timeout"
		}
		return urlErr.Err.Error()
	}
	return "request failed"
}
