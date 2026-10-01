package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureRequest runs one Complete call against a fake Messages API and returns
// the JSON body the client sent.
func captureRequest(t *testing.T, cfg *ClaudeConfig) map[string]any {
	t.Helper()
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"thinking","thinking":""},{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	cfg.APIKey = "sk-ant-test"
	cfg.BaseURL = srv.URL
	cfg.MaxTokens = 1024
	out, err := NewClient(cfg, nil).Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ok" {
		t.Errorf("Complete returned %q, want only the text block", out)
	}
	return sent
}

// Claude Sonnet 5, Opus 4.7 and later reject a non-default temperature with a
// 400. Leaving it unset in config must keep it off the wire, or those models
// cannot be configured at all.
func TestCompleteOmitsUnsetTemperature(t *testing.T) {
	sent := captureRequest(t, &ClaudeConfig{ModelID: "claude-sonnet-5"})
	if _, ok := sent["temperature"]; ok {
		t.Errorf("temperature was sent although unset: %v", sent["temperature"])
	}
}

func TestCompleteSendsConfiguredTemperature(t *testing.T) {
	sent := captureRequest(t, &ClaudeConfig{ModelID: "claude-haiku-4-5", Temperature: 0.3})
	if sent["temperature"] != 0.3 {
		t.Errorf("temperature = %v, want 0.3", sent["temperature"])
	}
}
