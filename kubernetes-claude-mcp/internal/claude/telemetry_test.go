package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/telemetry"
)

// Text that must never leave the process in a span: what the server sent the
// model, what the model answered, and the credential it used.
const (
	promptMarker   = "PROMPT-pod-payments-7f9c-crashloop"
	systemMarker   = "SYSTEM-you-are-a-cluster-expert"
	responseMarker = "RESPONSE-restart-the-deployment"
	apiKeyMarker   = "sk-ant-test-KEY-MARKER"
)

// fakeMessagesAPI answers every request with status and body.
func fakeMessagesAPI(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const okBody = `{
  "id": "msg_01XYZ",
  "type": "message",
  "model": "claude-haiku-4-5-20251001",
  "content": [{"type": "text", "text": "` + responseMarker + `"}],
  "usage": {"input_tokens": 11, "output_tokens": 7, "cache_read_input_tokens": 100, "cache_creation_input_tokens": 20}
}`

// otlpEndpoint is a fake OTLP/HTTP receiver that keeps every body it is sent.
type otlpEndpoint struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newOTLPEndpoint(t *testing.T) *otlpEndpoint {
	t.Helper()
	e := &otlpEndpoint{}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		e.mu.Lock()
		e.bodies = append(e.bodies, string(body))
		e.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(e.Close)
	return e
}

func (e *otlpEndpoint) sent() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.bodies...)
}

// spanAttrs returns the attributes of the only span the endpoint received.
func (e *otlpEndpoint) spanAttrs(t *testing.T) map[string]string {
	t.Helper()
	bodies := e.sent()
	if len(bodies) != 1 {
		t.Fatalf("the OTLP endpoint got %d requests, want 1", len(bodies))
	}
	var req struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []struct {
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string `json:"stringValue"`
							IntValue    string `json:"intValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal([]byte(bodies[0]), &req); err != nil {
		t.Fatalf("the export request is not JSON: %v", err)
	}
	if len(req.ResourceSpans) != 1 || len(req.ResourceSpans[0].ScopeSpans) != 1 || len(req.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
		t.Fatalf("want exactly one span, got %s", bodies[0])
	}
	out := map[string]string{}
	for _, a := range req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes {
		out[a.Key] = a.Value.StringValue + a.Value.IntValue
	}
	return out
}

type silentLogger struct{}

func (silentLogger) Info(string, ...interface{}) {}
func (silentLogger) Warn(string, ...interface{}) {}

func newReporter(endpoint string) *telemetry.Reporter {
	return telemetry.New(&telemetry.Config{
		Endpoint: endpoint,
		Headers:  map[string]string{"Authorization": "Bearer otlp-token"},
		Timeout:  2 * time.Second,
		Resource: map[string]string{"service.name": "test"},
	}, silentLogger{}, "0.0.0")
}

func flush(t *testing.T, r *telemetry.Reporter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r.Shutdown(ctx)
	if ctx.Err() != nil {
		t.Fatal("the reporter did not shut down")
	}
}

func testClient(baseURL string) *Client {
	return NewClient(&ClaudeConfig{APIKey: apiKeyMarker, BaseURL: baseURL, ModelID: "claude-haiku-4-5", MaxTokens: 1024}, nil)
}

var testMessages = []Message{
	{Role: "system", Content: systemMarker},
	{Role: "user", Content: promptMarker},
}

func TestCompleteReportsTheCall(t *testing.T) {
	api := fakeMessagesAPI(t, http.StatusOK, okBody)
	otlp := newOTLPEndpoint(t)
	reporter := newReporter(otlp.URL)

	client := testClient(api.URL)
	client.SetCallReporter(reporter)
	out, err := client.Complete(WithAnalysis(context.Background(), AnalysisNamespace), testMessages)
	if err != nil || out != responseMarker {
		t.Fatalf("Complete = (%q, %v)", out, err)
	}
	flush(t, reporter)

	want := map[string]string{
		"gen_ai.operation.name": "chat",
		"gen_ai.provider.name":  "anthropic",
		"gen_ai.request.model":  "claude-haiku-4-5",
		"gen_ai.response.model": "claude-haiku-4-5-20251001",
		"gen_ai.response.id":    "msg_01XYZ",
		// The API's input_tokens (11) leaves cached tokens out; the
		// convention's count includes the 100 read and the 20 written.
		"gen_ai.usage.input_tokens":             "131",
		"gen_ai.usage.output_tokens":            "7",
		"gen_ai.usage.cache_read.input_tokens":  "100",
		"gen_ai.usage.cache_write.input_tokens": "20",
		telemetry.AnalysisAttribute:             "namespace",
	}
	got := otlp.spanAttrs(t)
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
	if len(got) != len(want) {
		t.Errorf("attributes = %v, want exactly %d", got, len(want))
	}
}

// The span is counts, names and times. Whatever the server sent the model and
// whatever the model said stays out of it, on success and on failure.
func TestNothingOfThePromptOrResponseIsReported(t *testing.T) {
	// An API error can quote the request back, so its message is a way for
	// prompt text to leak. Only its type may be read.
	errorBody := `{"type":"error","error":{"type":"invalid_request_error","message":"bad input: ` + promptMarker + `"}}`

	for name, api := range map[string]*httptest.Server{
		"success": fakeMessagesAPI(t, http.StatusOK, okBody),
		"error":   fakeMessagesAPI(t, http.StatusBadRequest, errorBody),
	} {
		otlp := newOTLPEndpoint(t)
		reporter := newReporter(otlp.URL)
		client := testClient(api.URL)
		client.SetCallReporter(reporter)
		_, _ = client.Complete(WithAnalysis(context.Background(), AnalysisResource), testMessages)
		flush(t, reporter)

		bodies := otlp.sent()
		if len(bodies) != 1 {
			t.Fatalf("%s: the OTLP endpoint got %d requests, want 1", name, len(bodies))
		}
		for _, marker := range []string{promptMarker, systemMarker, responseMarker, apiKeyMarker} {
			if strings.Contains(bodies[0], marker) {
				t.Errorf("%s: the export contains %q: %s", name, marker, bodies[0])
			}
		}
	}
}

func TestAFailedCallIsReportedAndReturnedUnchanged(t *testing.T) {
	body := `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`
	api := fakeMessagesAPI(t, http.StatusTooManyRequests, body)

	// What the caller gets with reporting off.
	_, plainErr := testClient(api.URL).Complete(context.Background(), testMessages)
	if plainErr == nil {
		t.Fatal("a 429 did not fail the call")
	}

	otlp := newOTLPEndpoint(t)
	reporter := newReporter(otlp.URL)
	client := testClient(api.URL)
	client.SetCallReporter(reporter)
	_, err := client.Complete(context.Background(), testMessages)
	if err == nil || err.Error() != plainErr.Error() {
		t.Errorf("error with reporting on = %v, want the same as with it off: %v", err, plainErr)
	}
	flush(t, reporter)

	got := otlp.spanAttrs(t)
	if got["error.type"] != "rate_limit_error" {
		t.Errorf("error.type = %q, want the API's error type", got["error.type"])
	}
	if _, ok := got["gen_ai.usage.input_tokens"]; ok {
		t.Error("usage is reported for a call that returned none")
	}
}

func TestErrorTypes(t *testing.T) {
	// Without an API error type, the HTTP status code.
	if got := apiErrorType(http.StatusBadGateway, []byte("<html>bad gateway</html>")); got != "502" {
		t.Errorf("apiErrorType = %q, want 502", got)
	}

	// No response at all: the connection failed.
	api := fakeMessagesAPI(t, http.StatusOK, okBody)
	baseURL := api.URL
	api.Close()
	otlp := newOTLPEndpoint(t)
	reporter := newReporter(otlp.URL)
	client := testClient(baseURL)
	client.SetCallReporter(reporter)
	if _, err := client.Complete(context.Background(), testMessages); err == nil {
		t.Fatal("a call to a closed server succeeded")
	}
	flush(t, reporter)
	if got := otlp.spanAttrs(t)["error.type"]; got != "connection_error" {
		t.Errorf("error.type = %q, want connection_error", got)
	}

	// The caller gave up.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := transportErrorType(ctx.Err()); got != "canceled" {
		t.Errorf("transportErrorType(canceled) = %q", got)
	}
	if got := transportErrorType(context.DeadlineExceeded); got != "timeout" {
		t.Errorf("transportErrorType(deadline) = %q", got)
	}
}

// An OTLP endpoint that is down, refusing or hanging must not change what a
// model call returns or how long it takes.
func TestABrokenEndpointDoesNotAffectTheCall(t *testing.T) {
	api := fakeMessagesAPI(t, http.StatusOK, okBody)

	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()

	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer refusing.Close()

	release := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer hanging.Close()
	defer close(release)

	for name, endpoint := range map[string]string{"down": downURL, "refusing": refusing.URL, "hanging": hanging.URL} {
		reporter := newReporter(endpoint)
		client := testClient(api.URL)
		client.SetCallReporter(reporter)

		start := time.Now()
		for i := 0; i < 5; i++ {
			out, err := client.Complete(context.Background(), testMessages)
			if err != nil || out != responseMarker {
				t.Errorf("%s endpoint: Complete = (%q, %v)", name, out, err)
			}
		}
		if took := time.Since(start); took > time.Second {
			t.Errorf("%s endpoint: 5 calls took %v", name, took)
		}

		// Shutdown gives up when its context ends, whatever the endpoint does.
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		reporter.Shutdown(ctx)
		cancel()
	}
}

type panickingReporter struct{ calls int }

func (p *panickingReporter) ReportModelCall(*telemetry.ModelCall) {
	p.calls++
	panic("reporter bug")
}

func TestAReporterThatPanicsDoesNotAffectTheCall(t *testing.T) {
	api := fakeMessagesAPI(t, http.StatusOK, okBody)
	reporter := &panickingReporter{}
	client := testClient(api.URL)
	client.SetCallReporter(reporter)

	out, err := client.Complete(context.Background(), testMessages)
	if err != nil || out != responseMarker {
		t.Errorf("Complete = (%q, %v)", out, err)
	}
	if reporter.calls != 1 {
		t.Errorf("the reporter was called %d times, want 1", reporter.calls)
	}
}

// With no reporter set, which is how the client is built unless an OTLP
// endpoint is configured, a call makes exactly one HTTP request: its own.
func TestNothingIsReportedByDefault(t *testing.T) {
	var requests int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(okBody))
	}))
	defer api.Close()

	client := testClient(api.URL)
	if client.reporter != nil {
		t.Fatal("a new client has a reporter")
	}
	if _, err := client.Complete(context.Background(), testMessages); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Errorf("the call made %d requests, want 1", requests)
	}
}

func TestAnalysisIsOnlyWhatTheCodeNamed(t *testing.T) {
	if got := analysisFrom(context.Background()); got != "" {
		t.Errorf("analysisFrom(background) = %q, want empty", got)
	}
	if got := analysisFrom(WithAnalysis(context.Background(), AnalysisTroubleshoot)); got != "troubleshoot" {
		t.Errorf("analysisFrom = %q", got)
	}
}
