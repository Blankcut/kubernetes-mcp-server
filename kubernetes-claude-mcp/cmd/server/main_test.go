package main

import "testing"

// LOG_LEVEL used to be overwritten with the flag's hardcoded "info" default
// before the logger read it, so setting it did nothing.
func TestDefaultLogLevelReadsEnvironment(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	if got := defaultLogLevel(); got != "debug" {
		t.Errorf("defaultLogLevel() = %q with LOG_LEVEL=debug, want debug", got)
	}
	t.Setenv("LOG_LEVEL", "")
	if got := defaultLogLevel(); got != "info" {
		t.Errorf("defaultLogLevel() = %q with LOG_LEVEL unset, want info", got)
	}
}
