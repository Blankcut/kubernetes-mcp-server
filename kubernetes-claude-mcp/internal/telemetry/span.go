package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// AnalysisAttribute names what the server was doing when it made a model call
// (see ModelCall.Analysis). It is the one attribute that is this project's own
// rather than part of the OpenTelemetry GenAI conventions.
const AnalysisAttribute = "kubernetes_mcp_server.analysis"

// ModelCall is one finished call to the model: counts, names and times. It
// has no field that could hold a prompt, a response or cluster data, and none
// may be added.
type ModelCall struct {
	Start time.Time
	End   time.Time

	// Provider is the GenAI provider name, for example "anthropic".
	Provider string
	// RequestModel is the model asked for; ResponseModel the one that answered.
	RequestModel  string
	ResponseModel string
	// ResponseID is the provider's ID for the response.
	ResponseID string

	// Usage is nil when the call ended without reporting any.
	Usage *Usage

	// ErrorType is empty for a call that succeeded. Otherwise it is a short,
	// fixed class of failure: the provider's error type, an HTTP status code,
	// "timeout".
	ErrorType string

	// Analysis is which of the server's analyses the call was made for. It is
	// one of a fixed set of names chosen in code, never text from a request.
	Analysis string
}

// Usage is token usage as the provider's API counts it: InputTokens does not
// include tokens read from or written to the prompt cache.
type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// The types below are the part of the OTLP/JSON trace export request this
// package writes. In OTLP/JSON, IDs are hex and 64-bit integers are strings.

type exportRequest struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource   resource     `json:"resource"`
	ScopeSpans []scopeSpans `json:"scopeSpans"`
}

type resource struct {
	Attributes []keyValue `json:"attributes"`
}

type scopeSpans struct {
	Scope scope  `json:"scope"`
	Spans []span `json:"spans"`
}

type scope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type span struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	Name              string     `json:"name"`
	Kind              int        `json:"kind"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	EndTimeUnixNano   string     `json:"endTimeUnixNano"`
	Attributes        []keyValue `json:"attributes"`
	Status            status     `json:"status"`
}

type status struct {
	Code int `json:"code,omitempty"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type anyValue struct {
	StringValue *string `json:"stringValue,omitempty"`
	IntValue    *string `json:"intValue,omitempty"`
}

const (
	spanKindClient  = 3
	statusCodeError = 2
)

func stringAttr(key, value string) keyValue {
	return keyValue{Key: key, Value: anyValue{StringValue: &value}}
}

func intAttr(key string, value int64) keyValue {
	s := strconv.FormatInt(value, 10)
	return keyValue{Key: key, Value: anyValue{IntValue: &s}}
}

// buildSpan writes one model call in the OpenTelemetry GenAI conventions.
func buildSpan(call *ModelCall) span {
	attrs := []keyValue{stringAttr("gen_ai.operation.name", "chat")}
	add := func(key, value string) {
		if value != "" {
			attrs = append(attrs, stringAttr(key, value))
		}
	}
	add("gen_ai.provider.name", call.Provider)
	add("gen_ai.request.model", call.RequestModel)
	add("gen_ai.response.model", call.ResponseModel)
	add("gen_ai.response.id", call.ResponseID)
	if u := call.Usage; u != nil {
		// The convention's input count includes cached tokens; the provider's
		// leaves them out.
		attrs = append(attrs,
			intAttr("gen_ai.usage.input_tokens", u.InputTokens+u.CacheReadTokens+u.CacheWriteTokens),
			intAttr("gen_ai.usage.output_tokens", u.OutputTokens))
		if u.CacheReadTokens > 0 {
			attrs = append(attrs, intAttr("gen_ai.usage.cache_read.input_tokens", u.CacheReadTokens))
		}
		if u.CacheWriteTokens > 0 {
			attrs = append(attrs, intAttr("gen_ai.usage.cache_write.input_tokens", u.CacheWriteTokens))
		}
	}
	add("error.type", call.ErrorType)
	add(AnalysisAttribute, call.Analysis)

	name := "chat"
	if call.RequestModel != "" {
		name += " " + call.RequestModel
	}

	s := span{
		TraceID:           randomHex(16),
		SpanID:            randomHex(8),
		Name:              name,
		Kind:              spanKindClient,
		StartTimeUnixNano: strconv.FormatInt(call.Start.UnixNano(), 10),
		EndTimeUnixNano:   strconv.FormatInt(call.End.UnixNano(), 10),
		Attributes:        attrs,
	}
	if call.ErrorType != "" {
		s.Status.Code = statusCodeError
	}
	return s
}

// randomHex returns n random bytes as hex. crypto/rand.Read does not fail.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
