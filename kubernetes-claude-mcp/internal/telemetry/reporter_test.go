package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// secretToken stands in for the bearer token an operator configures. No log
// line may ever contain it.
const secretToken = "tok-3f9a1c-do-not-log"

// captureLogger records what the Reporter logs.
type captureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *captureLogger) record(level, msg string, kv []interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, level+" "+msg+" "+fmt.Sprint(kv...))
}

func (l *captureLogger) Info(msg string, kv ...interface{}) { l.record("INFO", msg, kv) }
func (l *captureLogger) Warn(msg string, kv ...interface{}) { l.record("WARN", msg, kv) }

func (l *captureLogger) warnings() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, line := range l.lines {
		if strings.HasPrefix(line, "WARN") {
			out = append(out, line)
		}
	}
	return out
}

func (l *captureLogger) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// received is one request a fake OTLP endpoint got.
type received struct {
	header http.Header
	body   []byte
}

// fakeEndpoint is an OTLP/HTTP receiver that answers each request with the
// next status in statuses (the last one repeats), and keeps what it was sent.
type fakeEndpoint struct {
	*httptest.Server
	mu       sync.Mutex
	requests []received
}

func newFakeEndpoint(t *testing.T, statuses ...int) *fakeEndpoint {
	t.Helper()
	if len(statuses) == 0 {
		statuses = []int{http.StatusOK}
	}
	f := &fakeEndpoint{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		n := len(f.requests)
		f.requests = append(f.requests, received{header: r.Header.Clone(), body: body})
		f.mu.Unlock()
		if n >= len(statuses) {
			n = len(statuses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statuses[n])
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeEndpoint) got() []received {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]received(nil), f.requests...)
}

func testConfig(endpoint string) *Config {
	return &Config{
		Endpoint: endpoint,
		Headers:  map[string]string{"Authorization": "Bearer " + secretToken},
		Timeout:  2 * time.Second,
		Resource: map[string]string{"service.name": "test-service", "k8s.namespace.name": "tools"},
	}
}

// quick starts a Reporter that batches for an hour, so that only Shutdown
// delivers, and retries without a pause.
func quick(cfg *Config, logger Logger) *Reporter {
	return newReporter(cfg, logger, "9.9.9", time.Hour, time.Millisecond, time.Minute)
}

func shutdown(t *testing.T, r *Reporter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r.Shutdown(ctx)
	if ctx.Err() != nil {
		t.Fatal("Shutdown did not finish")
	}
}

func sampleCall() *ModelCall {
	start := time.Unix(1_700_000_000, 0)
	return &ModelCall{
		Start:         start,
		End:           start.Add(1500 * time.Millisecond),
		Provider:      "anthropic",
		RequestModel:  "claude-haiku-4-5",
		ResponseModel: "claude-haiku-4-5-20251001",
		ResponseID:    "msg_01abc",
		Usage:         &Usage{InputTokens: 11, OutputTokens: 7, CacheReadTokens: 100, CacheWriteTokens: 20},
		Analysis:      "namespace",
	}
}

// decoded is the OTLP/JSON request as a receiver reads it.
type decoded struct {
	ResourceSpans []struct {
		Resource struct {
			Attributes []rawAttr `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Scope struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"scope"`
			Spans []struct {
				TraceID           string    `json:"traceId"`
				SpanID            string    `json:"spanId"`
				Name              string    `json:"name"`
				Kind              int       `json:"kind"`
				StartTimeUnixNano string    `json:"startTimeUnixNano"`
				EndTimeUnixNano   string    `json:"endTimeUnixNano"`
				Attributes        []rawAttr `json:"attributes"`
				Status            struct {
					Code int `json:"code"`
				} `json:"status"`
			} `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

type rawAttr struct {
	Key   string `json:"key"`
	Value struct {
		StringValue *string `json:"stringValue"`
		IntValue    *string `json:"intValue"`
	} `json:"value"`
}

func attrMap(t *testing.T, attrs []rawAttr) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, a := range attrs {
		switch {
		case a.Value.StringValue != nil:
			out[a.Key] = *a.Value.StringValue
		case a.Value.IntValue != nil:
			out[a.Key] = "int:" + *a.Value.IntValue
		default:
			t.Errorf("attribute %q has no value", a.Key)
		}
	}
	return out
}

func decode1(t *testing.T, body []byte) decoded {
	t.Helper()
	var d decoded
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("the export request is not JSON: %v", err)
	}
	if len(d.ResourceSpans) != 1 || len(d.ResourceSpans[0].ScopeSpans) != 1 {
		t.Fatalf("want one resource and one scope, got %s", body)
	}
	return d
}

func TestASpanCarriesTheGenAIAttributes(t *testing.T) {
	endpoint := newFakeEndpoint(t)
	r := quick(testConfig(endpoint.URL+"/v1/traces"), &captureLogger{})
	r.ReportModelCall(sampleCall())
	shutdown(t, r)

	requests := endpoint.got()
	if len(requests) != 1 {
		t.Fatalf("the endpoint got %d requests, want 1", len(requests))
	}
	req := requests[0]
	if got := req.header.Get("Authorization"); got != "Bearer "+secretToken {
		t.Errorf("Authorization header = %q", got)
	}
	if got := req.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}

	d := decode1(t, req.body)
	res := attrMap(t, d.ResourceSpans[0].Resource.Attributes)
	for key, want := range map[string]string{
		"service.name":       "test-service",
		"service.version":    "9.9.9",
		"k8s.namespace.name": "tools",
	} {
		if res[key] != want {
			t.Errorf("resource %s = %q, want %q", key, res[key], want)
		}
	}

	spans := d.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	s := spans[0]
	if len(s.TraceID) != 32 || len(s.SpanID) != 16 {
		t.Errorf("trace id %q / span id %q are not 16 and 8 bytes of hex", s.TraceID, s.SpanID)
	}
	if s.Name != "chat claude-haiku-4-5" || s.Kind != spanKindClient {
		t.Errorf("name %q kind %d, want a CLIENT span named for the operation and model", s.Name, s.Kind)
	}
	if s.StartTimeUnixNano != "1700000000000000000" || s.EndTimeUnixNano != "1700000001500000000" {
		t.Errorf("times = %s .. %s", s.StartTimeUnixNano, s.EndTimeUnixNano)
	}
	if s.Status.Code != 0 {
		t.Errorf("status code = %d for a call that succeeded", s.Status.Code)
	}

	want := map[string]string{
		"gen_ai.operation.name": "chat",
		"gen_ai.provider.name":  "anthropic",
		"gen_ai.request.model":  "claude-haiku-4-5",
		"gen_ai.response.model": "claude-haiku-4-5-20251001",
		"gen_ai.response.id":    "msg_01abc",
		// 11 uncached + 100 read from the cache + 20 written to it: the
		// convention's input count includes cached tokens.
		"gen_ai.usage.input_tokens":             "int:131",
		"gen_ai.usage.output_tokens":            "int:7",
		"gen_ai.usage.cache_read.input_tokens":  "int:100",
		"gen_ai.usage.cache_write.input_tokens": "int:20",
		AnalysisAttribute:                       "namespace",
	}
	got := attrMap(t, s.Attributes)
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
	if len(got) != len(want) {
		t.Errorf("attributes = %v, want exactly %d", got, len(want))
	}
}

func TestAFailedCallIsAnErrorSpanWithoutUsage(t *testing.T) {
	endpoint := newFakeEndpoint(t)
	r := quick(testConfig(endpoint.URL), &captureLogger{})
	call := sampleCall()
	call.Usage = nil
	call.ResponseModel, call.ResponseID = "", ""
	call.ErrorType = "rate_limit_error"
	r.ReportModelCall(call)
	shutdown(t, r)

	s := decode1(t, endpoint.got()[0].body).ResourceSpans[0].ScopeSpans[0].Spans[0]
	got := attrMap(t, s.Attributes)
	if got["error.type"] != "rate_limit_error" || s.Status.Code != statusCodeError {
		t.Errorf("error.type = %q, status code = %d", got["error.type"], s.Status.Code)
	}
	for key := range got {
		if strings.HasPrefix(key, "gen_ai.usage.") || strings.HasPrefix(key, "gen_ai.response.") {
			t.Errorf("%s is set on a call that returned no response", key)
		}
	}
}

func TestSpansAreBatched(t *testing.T) {
	endpoint := newFakeEndpoint(t)
	r := quick(testConfig(endpoint.URL), &captureLogger{})
	for i := 0; i < 5; i++ {
		r.ReportModelCall(sampleCall())
	}
	shutdown(t, r)

	requests := endpoint.got()
	if len(requests) != 1 {
		t.Fatalf("5 calls went out in %d requests, want 1", len(requests))
	}
	if n := len(decode1(t, requests[0].body).ResourceSpans[0].ScopeSpans[0].Spans); n != 5 {
		t.Errorf("the batch holds %d spans, want 5", n)
	}
}

func TestABatchIsSentWithoutShutdown(t *testing.T) {
	endpoint := newFakeEndpoint(t)
	r := newReporter(testConfig(endpoint.URL), &captureLogger{}, "", 20*time.Millisecond, time.Millisecond, time.Minute)
	defer shutdown(t, r)
	r.ReportModelCall(sampleCall())

	deadline := time.Now().Add(5 * time.Second)
	for len(endpoint.got()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the span was not sent after the batching window")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAnEndpointThatIsDownCostsTheCallerNothing(t *testing.T) {
	endpoint := newFakeEndpoint(t)
	url := endpoint.URL
	endpoint.Close() // nothing listens here any more

	logger := &captureLogger{}
	r := quick(testConfig(url), logger)

	start := time.Now()
	for i := 0; i < 100; i++ {
		r.ReportModelCall(sampleCall())
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Errorf("reporting 100 calls to a dead endpoint took %v", took)
	}
	shutdown(t, r)

	warnings := logger.warnings()
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want exactly 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "could not be delivered") {
		t.Errorf("warning = %q", warnings[0])
	}
	if strings.Contains(logger.all(), secretToken) {
		t.Errorf("the log contains the token: %s", logger.all())
	}
}

func TestAnEndpointThatHangsCostsTheCallerNothing(t *testing.T) {
	release := make(chan struct{})
	hung := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer hung.Close()
	defer close(release)

	cfg := testConfig(hung.URL)
	cfg.Timeout = 50 * time.Millisecond
	logger := &captureLogger{}
	// A 1ms batching window, so the sender is stuck in a request while the
	// calls below are reported.
	r := newReporter(cfg, logger, "", time.Millisecond, time.Millisecond, time.Minute)

	start := time.Now()
	for i := 0; i < queueSize*3; i++ {
		r.ReportModelCall(sampleCall())
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Errorf("reporting to a hanging endpoint took %v", took)
	}
	shutdown(t, r)

	joined := strings.Join(logger.warnings(), "\n")
	if !strings.Contains(joined, "timeout") {
		t.Errorf("warnings = %q, want the timeout named", joined)
	}
	if strings.Contains(logger.all(), secretToken) {
		t.Errorf("the log contains the token: %s", logger.all())
	}
}

func TestTheQueueIsBounded(t *testing.T) {
	release := make(chan struct{})
	var hits atomic.Int64
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		hits.Add(1)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	cfg := testConfig(slow.URL)
	cfg.Timeout = 10 * time.Second
	logger := &captureLogger{}
	r := newReporter(cfg, logger, "", time.Millisecond, time.Millisecond, 0)

	// One span gets the sender stuck in a request; then more spans arrive
	// than the queue holds.
	r.ReportModelCall(sampleCall())
	for hits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	const extra = 100
	for i := 0; i < queueSize+extra; i++ {
		r.ReportModelCall(sampleCall())
	}
	if n := r.overflow.Load(); n != extra {
		t.Errorf("%d spans were dropped, want the %d past the queue's size", n, extra)
	}

	close(release)
	shutdown(t, r)
	if !strings.Contains(strings.Join(logger.warnings(), "\n"), "queue was full") {
		t.Errorf("warnings = %v, want the dropped spans reported", logger.warnings())
	}
}

func TestARefusalIsNotRetriedAndDoesNotLogTheToken(t *testing.T) {
	endpoint := newFakeEndpoint(t, http.StatusUnauthorized)
	logger := &captureLogger{}
	r := quick(testConfig(endpoint.URL), logger)
	r.ReportModelCall(sampleCall())
	shutdown(t, r)

	if n := len(endpoint.got()); n != 1 {
		t.Errorf("a 401 was sent %d times, want 1", n)
	}
	warnings := logger.warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "refused") || !strings.Contains(warnings[0], "401") {
		t.Errorf("warnings = %v, want one naming the refusal and its status", warnings)
	}
	if strings.Contains(logger.all(), secretToken) {
		t.Errorf("the log contains the token: %s", logger.all())
	}
}

func TestAServerErrorIsRetriedOnce(t *testing.T) {
	endpoint := newFakeEndpoint(t, http.StatusServiceUnavailable, http.StatusOK)
	logger := &captureLogger{}
	r := quick(testConfig(endpoint.URL), logger)
	r.ReportModelCall(sampleCall())
	shutdown(t, r)

	requests := endpoint.got()
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want the export and one retry", len(requests))
	}
	if !bytes.Equal(requests[0].body, requests[1].body) {
		t.Error("the retry is not the same export: a receiver could count the span twice")
	}
	if len(logger.warnings()) != 0 {
		t.Errorf("warnings = %v, want none for an export that got through", logger.warnings())
	}

	// And only once: a receiver that stays down gets two requests, not more.
	down := newFakeEndpoint(t, http.StatusServiceUnavailable)
	r = quick(testConfig(down.URL), logger)
	r.ReportModelCall(sampleCall())
	shutdown(t, r)
	if n := len(down.got()); n != 2 {
		t.Errorf("a failing receiver got %d requests, want 2", n)
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	elsewhere := newFakeEndpoint(t)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	defer redirecting.Close()

	r := quick(testConfig(redirecting.URL), &captureLogger{})
	r.ReportModelCall(sampleCall())
	shutdown(t, r)

	if n := len(elsewhere.got()); n != 0 {
		t.Errorf("the export, with its headers, followed a redirect to another host (%d requests)", n)
	}
}

func TestWarningsAreRateLimited(t *testing.T) {
	endpoint := newFakeEndpoint(t, http.StatusUnauthorized)
	logger := &captureLogger{}
	// Every span is its own batch, and every batch is refused.
	r := newReporter(testConfig(endpoint.URL), logger, "", time.Millisecond, time.Millisecond, time.Hour)
	for i := 0; i < 5; i++ {
		r.ReportModelCall(sampleCall())
		time.Sleep(10 * time.Millisecond)
	}
	shutdown(t, r)

	if n := len(endpoint.got()); n < 2 {
		t.Fatalf("the endpoint got %d requests; the test needs several refusals", n)
	}
	if n := len(logger.warnings()); n != 1 {
		t.Errorf("got %d warnings for repeated refusals, want 1: %v", n, logger.warnings())
	}
}

func TestANilOrStoppedReporterIsSafe(t *testing.T) {
	var none *Reporter
	none.ReportModelCall(sampleCall())
	none.Shutdown(context.Background())

	endpoint := newFakeEndpoint(t)
	r := quick(testConfig(endpoint.URL), &captureLogger{})
	r.ReportModelCall(nil)
	shutdown(t, r)
	shutdown(t, r) // a second Shutdown is harmless
	r.ReportModelCall(sampleCall())

	if n := len(endpoint.got()); n != 0 {
		t.Errorf("the endpoint got %d requests, want none", n)
	}
}

func TestFromEnv(t *testing.T) {
	for _, name := range []string{
		"OTEL_SDK_DISABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_HEADERS",
		"OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
		"OTEL_EXPORTER_OTLP_TIMEOUT", "OTEL_EXPORTER_OTLP_TRACES_TIMEOUT",
		"OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES",
	} {
		t.Setenv(name, "")
	}

	// Nothing set: no Reporter, and nothing logged.
	logger := &captureLogger{}
	if r := FromEnv(logger, "1.0.0"); r != nil {
		t.Fatal("a Reporter was built with no endpoint set")
	}
	if logger.all() != "" {
		t.Errorf("logged %q with reporting off", logger.all())
	}

	// Set, but unusable: no Reporter, and one warning that says why.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	if r := FromEnv(logger, "1.0.0"); r != nil {
		t.Fatal("a Reporter was built for gRPC, which is not supported")
	}
	if n := len(logger.warnings()); n != 1 {
		t.Errorf("got %d warnings, want 1", n)
	}

	// Set: a Reporter, and a startup line that names the headers but not
	// their values.
	endpoint := newFakeEndpoint(t)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer "+secretToken)
	logger = &captureLogger{}
	r := FromEnv(logger, "1.0.0")
	if r == nil {
		t.Fatal("no Reporter was built with an endpoint set")
	}
	r.ReportModelCall(sampleCall())
	shutdown(t, r)

	requests := endpoint.got()
	if len(requests) != 1 {
		t.Fatalf("the endpoint got %d requests, want 1", len(requests))
	}
	if got := requests[0].header.Get("Authorization"); got != "Bearer "+secretToken {
		t.Errorf("Authorization header = %q", got)
	}
	if !strings.Contains(logger.all(), "Authorization") || strings.Contains(logger.all(), secretToken) {
		t.Errorf("startup log = %q, want the header named and its value absent", logger.all())
	}
}
