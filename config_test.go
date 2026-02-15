package main

import (
	"os"
	"testing"
)

func TestEnvOr(t *testing.T) {
	if got := envOr("PINCHTAB_TEST_UNSET_12345", "fallback"); got != "fallback" {
		t.Errorf("expected fallback, got %s", got)
	}

	_ = os.Setenv("PINCHTAB_TEST_SET", "value")
	defer func() { _ = os.Unsetenv("PINCHTAB_TEST_SET") }()
	if got := envOr("PINCHTAB_TEST_SET", "fallback"); got != "value" {
		t.Errorf("expected value, got %s", got)
	}
}

func TestHomeDir(t *testing.T) {
	h := homeDir()
	if h == "" {
		t.Error("homeDir returned empty string")
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are what handlers expect
	if actionClick != "click" {
		t.Error("actionClick mismatch")
	}
	if tabActionNew != "new" {
		t.Error("tabActionNew mismatch")
	}
	if maxBodySize != 1<<20 {
		t.Error("maxBodySize should be 1MB")
	}
}
