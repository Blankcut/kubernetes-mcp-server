package logging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// newBufferLogger returns a Logger that writes JSON lines to the returned
// buffer at every level, with the field names NewLogger uses.
func newBufferLogger(t *testing.T, opts ...zap.Option) (*Logger, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = ""
	encoderConfig.MessageKey = "msg"
	encoderConfig.CallerKey = "caller"
	encoderConfig.NameKey = "logger"
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(&buf), zapcore.DebugLevel)

	return newLogger(zap.New(core, append([]zap.Option{zap.AddCaller()}, opts...)...)), &buf
}

// entries decodes every JSON line written so far.
func entries(t *testing.T, buf *bytes.Buffer) []map[string]interface{} {
	t.Helper()

	var out []map[string]interface{}
	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	for scanner.Scan() {
		var entry map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("log line is not JSON: %v: %s", err, scanner.Text())
		}
		out = append(out, entry)
	}
	return out
}

func onlyEntry(t *testing.T, buf *bytes.Buffer) map[string]interface{} {
	t.Helper()

	got := entries(t, buf)
	if len(got) != 1 {
		t.Fatalf("got %d log entries, want 1: %s", len(got), buf)
	}
	return got[0]
}

// The production symptom was "msg":"HTTP requestmethodGETpath/api/v1/...":
// the pairs had been fmt.Sprint-ed into the message.
func TestKeyValueMethodsLogStructuredFields(t *testing.T) {
	levels := []struct {
		level string
		log   func(l *Logger, msg string, kv ...interface{})
	}{
		{"debug", (*Logger).Debug},
		{"info", (*Logger).Info},
		{"warn", (*Logger).Warn},
		{"error", (*Logger).Error},
		{"dpanic", (*Logger).DPanic},
	}

	for _, tt := range levels {
		t.Run(tt.level, func(t *testing.T) {
			logger, buf := newBufferLogger(t)

			tt.log(logger, "HTTP request", "method", "GET", "path", "/api/v1/argocd/applications", "status", 200)

			entry := onlyEntry(t, buf)
			if entry["msg"] != "HTTP request" {
				t.Errorf("msg = %q, want %q", entry["msg"], "HTTP request")
			}
			if entry["level"] != tt.level {
				t.Errorf("level = %q, want %q", entry["level"], tt.level)
			}
			if entry["method"] != "GET" || entry["path"] != "/api/v1/argocd/applications" || entry["status"] != float64(200) {
				t.Errorf("fields missing or wrong: %v", entry)
			}
			// The wrapper adds a stack frame; caller must still be the call site.
			if caller, _ := entry["caller"].(string); !strings.HasPrefix(caller, "logging/logging_test.go:") {
				t.Errorf("caller = %q, want this test file", entry["caller"])
			}
		})
	}
}

func TestPanicAndFatalLogStructuredFields(t *testing.T) {
	levels := []struct {
		level string
		log   func(l *Logger, msg string, kv ...interface{})
	}{
		{"panic", (*Logger).Panic},
		// WriteThenPanic stands in for os.Exit so the test can observe Fatal.
		{"fatal", (*Logger).Fatal},
	}

	for _, tt := range levels {
		t.Run(tt.level, func(t *testing.T) {
			logger, buf := newBufferLogger(t, zap.WithFatalHook(zapcore.WriteThenPanic))

			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("%s did not panic", tt.level)
					}
				}()
				tt.log(logger, "Failed to load configuration", "error", "no such file")
			}()

			entry := onlyEntry(t, buf)
			if entry["msg"] != "Failed to load configuration" || entry["error"] != "no such file" {
				t.Errorf("entry = %v, want the message and an error field", entry)
			}
		})
	}
}

func TestDerivedLoggersKeepStructuredFields(t *testing.T) {
	logger, buf := newBufferLogger(t)

	logger.Named("argocd").With("component", "authz").Info("Probe finished", "state", "authorized")

	entry := onlyEntry(t, buf)
	if entry["msg"] != "Probe finished" {
		t.Errorf("msg = %q", entry["msg"])
	}
	if entry["logger"] != "argocd" || entry["component"] != "authz" || entry["state"] != "authorized" {
		t.Errorf("entry = %v, want name, With field and call field", entry)
	}
	if caller, _ := entry["caller"].(string); !strings.HasPrefix(caller, "logging/logging_test.go:") {
		t.Errorf("caller = %q, want this test file", entry["caller"])
	}
}

// Methods the wrapper does not override still come from the SugaredLogger
// and must keep reporting their own call site.
func TestPromotedSugaredMethodsAreUnchanged(t *testing.T) {
	logger, buf := newBufferLogger(t)

	logger.Infow("structured", "k", "v")
	logger.Infof("formatted %d", 7)

	got := entries(t, buf)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %s", len(got), buf)
	}
	if got[0]["msg"] != "structured" || got[0]["k"] != "v" {
		t.Errorf("Infow entry = %v", got[0])
	}
	if got[1]["msg"] != "formatted 7" {
		t.Errorf("Infof entry = %v", got[1])
	}
	for _, entry := range got {
		if caller, _ := entry["caller"].(string); !strings.HasPrefix(caller, "logging/logging_test.go:") {
			t.Errorf("caller = %q, want this test file", entry["caller"])
		}
	}
}

// A malformed call must neither panic nor lose the well-formed fields, and
// what was dropped must be visible rather than silently discarded.
func TestMalformedKeyValuesDoNotPanic(t *testing.T) {
	tests := []struct {
		name        string
		args        []interface{}
		wantField   string // a well-formed pair that must survive
		wantProblem string // message of zap's separate error entry
	}{
		{
			name:        "dangling key",
			args:        []interface{}{"method", "GET", "status"},
			wantField:   "method",
			wantProblem: "Ignored key without a value.",
		},
		{
			name:        "non-string key",
			args:        []interface{}{"method", "GET", 42, "value"},
			wantField:   "method",
			wantProblem: "Ignored key-value pairs with non-string keys.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := newBufferLogger(t)

			logger.Info("HTTP request", tt.args...)

			var main, problem map[string]interface{}
			for _, entry := range entries(t, buf) {
				switch entry["msg"] {
				case "HTTP request":
					main = entry
				case tt.wantProblem:
					problem = entry
				}
			}
			if main == nil {
				t.Fatalf("the original entry was not logged: %s", buf)
			}
			if _, ok := main[tt.wantField]; !ok {
				t.Errorf("well-formed field %q was lost: %v", tt.wantField, main)
			}
			if problem == nil {
				t.Errorf("no %q entry reporting the dropped arguments: %s", tt.wantProblem, buf)
			}
		})
	}
}
