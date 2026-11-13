// Package telemetry reports each model call the server makes as one
// OpenTelemetry span, sent over OTLP/HTTP as JSON: which model, how many
// tokens, how long it took and whether it failed.
//
// Rules this package keeps:
//
//   - It can never fail or slow a model call. Reporting is a non-blocking put
//     on a bounded queue; a background goroutine sends batches with a short
//     timeout. An endpoint that is down, slow or refusing costs the call
//     nothing, and is logged at most once a minute.
//   - It does nothing unless an OTLP endpoint is configured.
//   - A span carries counts, names and times only. Never a prompt, a response,
//     or anything read from the cluster.
//   - Header values (they usually carry a token) are never logged.
//
// It is configured with the standard OpenTelemetry environment variables, and
// written against the standard library alone: the OpenTelemetry SDK with its
// OTLP exporter would grow the stripped linux/amd64 binary from 31 MB to 54 MB
// and add gRPC to the dependency tree, for one kind of span.
package telemetry

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultServiceName is the service.name reported when neither
// OTEL_SERVICE_NAME nor OTEL_RESOURCE_ATTRIBUTES gives one.
const DefaultServiceName = "kubernetes-mcp-server"

// defaultTimeout bounds one export request. It is shorter than the 10s the
// OTLP specification suggests: nothing waits on an export except shutdown.
const defaultTimeout = 5 * time.Second

// namespaceFile is where Kubernetes tells a pod which namespace it runs in.
const namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// Config is where and how spans are sent.
type Config struct {
	// Endpoint is the full URL spans are POSTed to.
	Endpoint string
	// Headers are sent with every export. Their values are never logged.
	Headers map[string]string
	// Timeout bounds one export request.
	Timeout time.Duration
	// Resource describes this process: service.name, and on Kubernetes the
	// namespace and pod it runs as.
	Resource map[string]string
}

// ConfigFromEnv reads the standard OpenTelemetry variables through getenv.
//
// It returns nil with a nil error when no endpoint is set (or
// OTEL_SDK_DISABLED is true): reporting is off, which is the default. It
// returns an error when an endpoint is set but cannot be used, so the caller
// can say so once and carry on without reporting.
//
// Variables read: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT (used as given), else
// OTEL_EXPORTER_OTLP_ENDPOINT (with /v1/traces appended); the matching
// _HEADERS, _TIMEOUT and _PROTOCOL; OTEL_SERVICE_NAME and
// OTEL_RESOURCE_ATTRIBUTES.
func ConfigFromEnv(getenv func(string) string) (*Config, error) {
	if strings.EqualFold(strings.TrimSpace(getenv("OTEL_SDK_DISABLED")), "true") {
		return nil, nil
	}

	endpoint := strings.TrimSpace(getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	if endpoint == "" {
		base := strings.TrimSpace(getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
		if base == "" {
			return nil, nil
		}
		endpoint = strings.TrimRight(base, "/") + "/v1/traces"
	}

	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		// The value itself is not echoed: a URL can carry credentials.
		return nil, fmt.Errorf("the OTLP endpoint is not an http(s) URL")
	}

	// Spans are always sent as OTLP/HTTP JSON, which every OTLP/HTTP receiver
	// accepts on the same URL as protobuf. gRPC is a different port and wire
	// format, so asking for it cannot be honored.
	protocol := firstSet(getenv, "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_EXPORTER_OTLP_PROTOCOL")
	if strings.EqualFold(protocol, "grpc") {
		return nil, fmt.Errorf("OTLP over gRPC is not supported: point the endpoint at an OTLP/HTTP receiver (usually port 4318) and unset the protocol or set it to http/json")
	}

	cfg := &Config{
		Endpoint: endpoint,
		Headers:  parsePairs(firstSet(getenv, "OTEL_EXPORTER_OTLP_TRACES_HEADERS", "OTEL_EXPORTER_OTLP_HEADERS")),
		Timeout:  defaultTimeout,
		Resource: parsePairs(getenv("OTEL_RESOURCE_ATTRIBUTES")),
	}

	if raw := firstSet(getenv, "OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "OTEL_EXPORTER_OTLP_TIMEOUT"); raw != "" {
		if ms, convErr := strconv.Atoi(raw); convErr == nil && ms > 0 {
			cfg.Timeout = time.Duration(ms) * time.Millisecond
		}
	}

	if name := strings.TrimSpace(getenv("OTEL_SERVICE_NAME")); name != "" {
		cfg.Resource["service.name"] = name
	}
	if cfg.Resource["service.name"] == "" {
		cfg.Resource["service.name"] = DefaultServiceName
	}

	return cfg, nil
}

// DetectKubernetes adds the namespace and pod this process runs as to the
// resource, when it runs in a pod and the configuration did not already say.
// A receiver that charges usage to a namespace needs it.
func (c *Config) DetectKubernetes(getenv func(string) string) {
	c.detectKubernetes(getenv, namespaceFile)
}

func (c *Config) detectKubernetes(getenv func(string) string, nsFile string) {
	raw, err := os.ReadFile(nsFile) //nolint:gosec // a fixed, well-known path
	if err != nil {
		return
	}
	namespace := strings.TrimSpace(string(raw))
	if namespace == "" {
		return
	}
	if c.Resource["k8s.namespace.name"] == "" {
		c.Resource["k8s.namespace.name"] = namespace
	}
	// In a pod the hostname is the pod's name.
	if pod := strings.TrimSpace(getenv("HOSTNAME")); pod != "" && c.Resource["k8s.pod.name"] == "" {
		c.Resource["k8s.pod.name"] = pod
	}
}

// displayEndpoint is the endpoint as it may be logged: no credentials, no
// query string.
func (c *Config) displayEndpoint() string {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// headerNames lists the configured headers without their values.
func (c *Config) headerNames() []string {
	names := make([]string, 0, len(c.Headers))
	for name := range c.Headers {
		names = append(names, name)
	}
	return names
}

func firstSet(getenv func(string) string, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// parsePairs reads "k1=v1,k2=v2", the format OTEL_EXPORTER_OTLP_HEADERS and
// OTEL_RESOURCE_ATTRIBUTES share. Keys and values are percent-decoded; one
// that does not decode is kept as written, so a value such as "Bearer abc"
// works with or without its space encoded. A member without "=" is skipped.
func parsePairs(raw string) map[string]string {
	out := map[string]string{}
	for _, member := range strings.Split(raw, ",") {
		key, value, found := strings.Cut(member, "=")
		if !found {
			continue
		}
		key = decode(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		out[key] = decode(strings.TrimSpace(value))
	}
	return out
}

func decode(s string) string {
	if decoded, err := url.PathUnescape(s); err == nil {
		return decoded
	}
	return s
}
