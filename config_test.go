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

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"", "(none)"},
		{"short", "***"},
		{"12345678", "***"},
		{"123456789", "1234...6789"},
		{"my-super-secret-token", "my-s...oken"},
	}
	for _, tt := range tests {
		if got := maskToken(tt.input); got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Port != "9867" {
		t.Errorf("default port = %q, want 9867", cfg.Port)
	}
	if !cfg.Headless {
		t.Error("default config should be headless")
	}
	if cfg.NoRestore {
		t.Error("default config should not have noRestore")
	}
	if cfg.TimeoutSec != 15 {
		t.Errorf("default timeout = %d, want 15", cfg.TimeoutSec)
	}
	if cfg.NavigateSec != 30 {
		t.Errorf("default navigate timeout = %d, want 30", cfg.NavigateSec)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	// Save original values
	origPort := port
	defer func() { port = origPort }()

	// Create a temp config file
	dir := t.TempDir()
	configPath := dir + "/config.json"
	os.WriteFile(configPath, []byte(`{"port":"7777"}`), 0644)

	// Ensure env vars don't interfere
	os.Unsetenv("BRIDGE_PORT")
	os.Setenv("BRIDGE_CONFIG", configPath)
	defer os.Unsetenv("BRIDGE_CONFIG")

	loadConfig()
	if port != "7777" {
		t.Errorf("loadConfig from file: port = %q, want 7777", port)
	}
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	origPort := port
	defer func() { port = origPort }()

	dir := t.TempDir()
	configPath := dir + "/config.json"
	os.WriteFile(configPath, []byte(`{"port":"7777"}`), 0644)

	// Set BRIDGE_PORT env — loadConfig should NOT override port from file
	os.Setenv("BRIDGE_CONFIG", configPath)
	os.Setenv("BRIDGE_PORT", "8888")
	defer os.Unsetenv("BRIDGE_CONFIG")
	defer os.Unsetenv("BRIDGE_PORT")

	// Simulate what happens: port was set at init from env, so set it manually
	port = "8888"
	loadConfig()
	// File says 7777, but env is set so loadConfig should leave port alone
	if port != "8888" {
		t.Errorf("env should override file: port = %q, want 8888", port)
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
