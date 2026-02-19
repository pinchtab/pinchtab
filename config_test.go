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

func TestEnvBoolOr(t *testing.T) {
	if got := envBoolOr("PINCHTAB_TEST_BOOL_UNSET", true); !got {
		t.Error("expected fallback true for unset env")
	}
	if got := envBoolOr("PINCHTAB_TEST_BOOL_UNSET", false); got {
		t.Error("expected fallback false for unset env")
	}

	t.Setenv("PINCHTAB_TEST_BOOL", "true")
	if got := envBoolOr("PINCHTAB_TEST_BOOL", false); !got {
		t.Error("expected true from env true")
	}

	t.Setenv("PINCHTAB_TEST_BOOL", "false")
	if got := envBoolOr("PINCHTAB_TEST_BOOL", true); got {
		t.Error("expected false from env false")
	}

	t.Setenv("PINCHTAB_TEST_BOOL", "1")
	if got := envBoolOr("PINCHTAB_TEST_BOOL", false); !got {
		t.Error("expected true from env 1")
	}

	t.Setenv("PINCHTAB_TEST_BOOL", "0")
	if got := envBoolOr("PINCHTAB_TEST_BOOL", true); got {
		t.Error("expected false from env 0")
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
		{"my-super-secret-cfg.Token", "my-s...oken"},
	}
	for _, tt := range tests {
		if got := maskToken(tt.input); got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	fc := defaultFileConfig()
	if fc.Port != "9867" {
		t.Errorf("default Port = %q, want 9867", fc.Port)
	}
	if fc.Headless == nil || !*fc.Headless {
		t.Error("default config should be headless")
	}
	if fc.NoRestore {
		t.Error("default config should not have NoRestore")
	}
	if fc.TimeoutSec != 15 {
		t.Errorf("default timeout = %d, want 15", fc.TimeoutSec)
	}
	if fc.NavigateSec != 30 {
		t.Errorf("default navigate timeout = %d, want 30", fc.NavigateSec)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {

	origPort := cfg.Port
	defer func() { cfg.Port = origPort }()

	dir := t.TempDir()
	configPath := dir + "/config.json"
	_ = os.WriteFile(configPath, []byte(`{"port":"7777"}`), 0644)

	t.Setenv("BRIDGE_PORT", "")
	t.Setenv("BRIDGE_CONFIG", configPath)

	loadConfig()
	if cfg.Port != "7777" {
		t.Errorf("loadConfig from file: port = %q, want 7777", cfg.Port)
	}
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	origPort := cfg.Port
	defer func() { cfg.Port = origPort }()

	dir := t.TempDir()
	configPath := dir + "/config.json"
	_ = os.WriteFile(configPath, []byte(`{"port":"7777"}`), 0644)

	t.Setenv("BRIDGE_CONFIG", configPath)
	t.Setenv("BRIDGE_PORT", "8888")

	cfg.Port = "8888"
	loadConfig()

	if cfg.Port != "8888" {
		t.Errorf("env should override file: port = %q, want 8888", cfg.Port)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"
	_ = os.WriteFile(configPath, []byte("{broken json!!!"), 0644)

	origPort := cfg.Port
	defer func() { cfg.Port = origPort }()

	t.Setenv("BRIDGE_CONFIG", configPath)
	t.Setenv("BRIDGE_PORT", "")
	cfg.Port = "9867"
	loadConfig()
	if cfg.Port != "9867" {
		t.Errorf("invalid config should not change port, got %q", cfg.Port)
	}
}

func TestConstants(t *testing.T) {

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
