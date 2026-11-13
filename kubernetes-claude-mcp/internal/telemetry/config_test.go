package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(pairs map[string]string) func(string) string {
	return func(name string) string { return pairs[name] }
}

func TestConfigIsOffUnlessAnEndpointIsSet(t *testing.T) {
	for name, pairs := range map[string]map[string]string{
		"nothing set": {},
		"headers only": {
			"OTEL_EXPORTER_OTLP_HEADERS": "Authorization=Bearer abc",
			"OTEL_SERVICE_NAME":          "x",
		},
		"blank endpoint": {"OTEL_EXPORTER_OTLP_ENDPOINT": "   "},
		"sdk disabled": {
			"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
			"OTEL_SDK_DISABLED":           "true",
		},
	} {
		cfg, err := ConfigFromEnv(env(pairs))
		if cfg != nil || err != nil {
			t.Errorf("%s: got (%v, %v), want reporting off with no error", name, cfg, err)
		}
	}
}

func TestConfigEndpoint(t *testing.T) {
	// The traces-specific variable is a full URL and is used as given.
	cfg, err := ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://collector:4318/custom/path",
		"OTEL_EXPORTER_OTLP_ENDPOINT":        "http://ignored:4318",
	}))
	if err != nil || cfg.Endpoint != "http://collector:4318/custom/path" {
		t.Errorf("traces endpoint: got (%+v, %v)", cfg, err)
	}

	// The general variable is a base URL: the signal path is appended.
	for _, base := range []string{"http://collector:4318", "http://collector:4318/"} {
		cfg, err = ConfigFromEnv(env(map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": base}))
		if err != nil || cfg.Endpoint != "http://collector:4318/v1/traces" {
			t.Errorf("base %q: got (%+v, %v)", base, cfg, err)
		}
	}
}

func TestConfigRefusesWhatCannotWork(t *testing.T) {
	_, err := ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "collector:4317",
	}))
	if err == nil {
		t.Error("an endpoint that is not an http(s) URL was accepted")
	}

	_, err = ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4317",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc",
	}))
	if err == nil || !strings.Contains(err.Error(), "gRPC") {
		t.Errorf("grpc protocol: got %v, want an error that names gRPC", err)
	}

	// A URL can carry credentials, so a rejected one is not repeated.
	_, err = ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "ftp://user:hunter2@collector/x",
	}))
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("bad endpoint: got %v, want an error that does not repeat the value", err)
	}
}

func TestConfigHeaders(t *testing.T) {
	cfg, err := ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
		"OTEL_EXPORTER_OTLP_HEADERS":  "Authorization=Bearer abc123==, X-Scope = team%20a ,novalue",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Headers["Authorization"]; got != "Bearer abc123==" {
		t.Errorf("Authorization = %q", got)
	}
	if got := cfg.Headers["X-Scope"]; got != "team a" {
		t.Errorf("X-Scope = %q, want the percent-decoded value", got)
	}
	if len(cfg.Headers) != 2 {
		t.Errorf("headers = %v, want the member without '=' skipped", cfg.headerNames())
	}

	// An encoded space reads the same as a literal one.
	cfg, _ = ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
		"OTEL_EXPORTER_OTLP_HEADERS":  "Authorization=Bearer%20abc123",
	}))
	if got := cfg.Headers["Authorization"]; got != "Bearer abc123" {
		t.Errorf("Authorization = %q", got)
	}

	// The traces-specific variable replaces the general one.
	cfg, _ = ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":       "http://collector:4318",
		"OTEL_EXPORTER_OTLP_HEADERS":        "Authorization=Bearer general",
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS": "Authorization=Bearer traces",
	}))
	if got := cfg.Headers["Authorization"]; got != "Bearer traces" {
		t.Errorf("Authorization = %q, want the traces-specific value", got)
	}
}

func TestConfigResourceAndTimeout(t *testing.T) {
	cfg, err := ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Resource["service.name"] != DefaultServiceName {
		t.Errorf("service.name = %q, want the default", cfg.Resource["service.name"])
	}
	if cfg.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want the default", cfg.Timeout)
	}

	cfg, _ = ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
		"OTEL_RESOURCE_ATTRIBUTES":    "service.name=from-attributes,deployment.environment.name=prod",
		"OTEL_EXPORTER_OTLP_TIMEOUT":  "1500",
	}))
	if cfg.Resource["service.name"] != "from-attributes" || cfg.Resource["deployment.environment.name"] != "prod" {
		t.Errorf("resource = %v", cfg.Resource)
	}
	if cfg.Timeout != 1500*time.Millisecond {
		t.Errorf("timeout = %v, want 1.5s", cfg.Timeout)
	}

	// OTEL_SERVICE_NAME wins over service.name in the attributes.
	cfg, _ = ConfigFromEnv(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
		"OTEL_RESOURCE_ATTRIBUTES":    "service.name=from-attributes",
		"OTEL_SERVICE_NAME":           "from-service-name",
	}))
	if cfg.Resource["service.name"] != "from-service-name" {
		t.Errorf("service.name = %q", cfg.Resource["service.name"])
	}
}

func TestDetectKubernetes(t *testing.T) {
	nsFile := filepath.Join(t.TempDir(), "namespace")
	if err := os.WriteFile(nsFile, []byte("tools\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pod := env(map[string]string{"HOSTNAME": "server-5d9c7-abcde"})

	cfg := &Config{Resource: map[string]string{}}
	cfg.detectKubernetes(pod, nsFile)
	if cfg.Resource["k8s.namespace.name"] != "tools" || cfg.Resource["k8s.pod.name"] != "server-5d9c7-abcde" {
		t.Errorf("resource = %v", cfg.Resource)
	}

	// What the configuration says is kept.
	cfg = &Config{Resource: map[string]string{"k8s.namespace.name": "explicit"}}
	cfg.detectKubernetes(pod, nsFile)
	if cfg.Resource["k8s.namespace.name"] != "explicit" {
		t.Errorf("k8s.namespace.name = %q, want the configured value", cfg.Resource["k8s.namespace.name"])
	}

	// Outside a pod there is no file, and nothing is guessed from HOSTNAME.
	cfg = &Config{Resource: map[string]string{}}
	cfg.detectKubernetes(pod, filepath.Join(t.TempDir(), "missing"))
	if len(cfg.Resource) != 0 {
		t.Errorf("resource = %v, want nothing detected outside a pod", cfg.Resource)
	}
}

func TestDisplayEndpointHasNoCredentialsOrQuery(t *testing.T) {
	cfg := &Config{Endpoint: "https://user:hunter2@collector.example:4318/v1/traces?api_key=hunter3"}
	got := cfg.displayEndpoint()
	if got != "https://collector.example:4318/v1/traces" {
		t.Errorf("displayEndpoint = %q", got)
	}
}
