package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnvOr(t *testing.T) {
	key := "PINCHTAB_TEST_ENV"
	fallback := "default"

	_ = os.Unsetenv(key)
	if got := envOr(key, fallback); got != fallback {
		t.Errorf("envOr() = %v, want %v", got, fallback)
	}

	val := "set"
	_ = os.Setenv(key, val)
	defer func() { _ = os.Unsetenv(key) }()
	if got := envOr(key, fallback); got != val {
		t.Errorf("envOr() = %v, want %v", got, val)
	}
}

func TestEnvIntOr(t *testing.T) {
	key := "PINCHTAB_TEST_INT"
	fallback := 42

	_ = os.Unsetenv(key)
	if got := envIntOr(key, fallback); got != fallback {
		t.Errorf("envIntOr() = %v, want %v", got, fallback)
	}

	_ = os.Setenv(key, "100")
	if got := envIntOr(key, fallback); got != 100 {
		t.Errorf("envIntOr() = %v, want %v", got, 100)
	}

	_ = os.Setenv(key, "invalid")
	if got := envIntOr(key, fallback); got != fallback {
		t.Errorf("envIntOr() = %v, want %v", got, fallback)
	}
}

func TestEnvBoolOr(t *testing.T) {
	key := "PINCHTAB_TEST_BOOL"
	fallback := true

	_ = os.Unsetenv(key)
	if got := envBoolOr(key, fallback); got != fallback {
		t.Errorf("envBoolOr() = %v, want %v", got, fallback)
	}

	tests := []struct {
		val  string
		want bool
	}{
		{"1", true}, {"true", true}, {"yes", true}, {"on", true},
		{"0", false}, {"false", false}, {"no", false}, {"off", false},
		{"garbage", true}, // should return fallback
	}

	for _, tt := range tests {
		_ = os.Setenv(key, tt.val)
		if got := envBoolOr(key, fallback); got != tt.want {
			t.Errorf("envBoolOr(%q) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		token string
		want  string
	}{
		{"", "(none)"},
		{"short", "***"},
		{"very-long-token-secret", "very...cret"},
	}

	for _, tt := range tests {
		if got := MaskToken(tt.token); got != tt.want {
			t.Errorf("MaskToken(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	_ = os.Unsetenv("PINCHTAB_PORT")
	_ = os.Unsetenv("PINCHTAB_BIND")
	_ = os.Unsetenv("BRIDGE_PORT")
	_ = os.Unsetenv("BRIDGE_BIND")
	_ = os.Unsetenv("CDP_URL")
	_ = os.Unsetenv("PINCHTAB_TOKEN")
	_ = os.Unsetenv("BRIDGE_TOKEN")
	_ = os.Unsetenv("PINCHTAB_ALLOW_EVALUATE")
	_ = os.Unsetenv("BRIDGE_ALLOW_EVALUATE")

	cfg := Load()
	if cfg.Port != "9867" {
		t.Errorf("default Port = %v, want 9867", cfg.Port)
	}
	if cfg.Bind != "127.0.0.1" {
		t.Errorf("default Bind = %v, want 127.0.0.1", cfg.Bind)
	}
	if cfg.AllowEvaluate {
		t.Errorf("default AllowEvaluate = %v, want false", cfg.AllowEvaluate)
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	_ = os.Setenv("PINCHTAB_PORT", "1234")
	_ = os.Setenv("PINCHTAB_ALLOW_EVALUATE", "1")
	defer func() {
		_ = os.Unsetenv("PINCHTAB_PORT")
		_ = os.Unsetenv("PINCHTAB_ALLOW_EVALUATE")
	}()

	cfg := Load()
	if cfg.Port != "1234" {
		t.Errorf("env Port = %v, want 1234", cfg.Port)
	}
	if !cfg.AllowEvaluate {
		t.Errorf("env AllowEvaluate = %v, want true", cfg.AllowEvaluate)
	}
}

func TestLegacyBridgeEnvFallback(t *testing.T) {
	_ = os.Unsetenv("PINCHTAB_PORT")
	_ = os.Unsetenv("PINCHTAB_ALLOW_EVALUATE")
	_ = os.Setenv("BRIDGE_PORT", "5555")
	_ = os.Setenv("BRIDGE_ALLOW_EVALUATE", "true")
	defer func() {
		_ = os.Unsetenv("BRIDGE_PORT")
		_ = os.Unsetenv("BRIDGE_ALLOW_EVALUATE")
	}()

	cfg := Load()
	if cfg.Port != "5555" {
		t.Errorf("legacy fallback Port = %v, want 5555", cfg.Port)
	}
	if !cfg.AllowEvaluate {
		t.Errorf("legacy fallback AllowEvaluate = %v, want true", cfg.AllowEvaluate)
	}
}

func TestPinchtabEnvTakesPrecedence(t *testing.T) {
	_ = os.Setenv("PINCHTAB_PORT", "7777")
	_ = os.Setenv("BRIDGE_PORT", "8888")
	defer func() {
		_ = os.Unsetenv("PINCHTAB_PORT")
		_ = os.Unsetenv("BRIDGE_PORT")
	}()

	cfg := Load()
	if cfg.Port != "7777" {
		t.Errorf("precedence Port = %v, want 7777 (PINCHTAB_ should win)", cfg.Port)
	}
}

func TestDefaultFileConfig(t *testing.T) {
	fc := DefaultFileConfig()
	if fc.Port != "9867" {
		t.Errorf("DefaultFileConfig.Port = %v, want 9867", fc.Port)
	}
	if *fc.Headless != true {
		t.Errorf("DefaultFileConfig.Headless = %v, want true", *fc.Headless)
	}
}

func TestLoadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	// Create a dummy config file
	configData := `{
		"port": "8888",
		"allowEvaluate": true,
		"headless": false,
		"timeoutSec": 60
	}`
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.Port != "8888" {
		t.Errorf("file Port = %v, want 8888", cfg.Port)
	}
	if cfg.AllowEvaluate != true {
		t.Errorf("file AllowEvaluate = %v, want true", cfg.AllowEvaluate)
	}
	if cfg.Headless != false {
		t.Errorf("file Headless = %v, want false", cfg.Headless)
	}
	if cfg.ActionTimeout != 60*time.Second {
		t.Errorf("file ActionTimeout = %v, want 60s", cfg.ActionTimeout)
	}
}

func TestListenAddr(t *testing.T) {
	cfg := &RuntimeConfig{Bind: "127.0.0.1", Port: "9867"}
	if got := cfg.ListenAddr(); got != "127.0.0.1:9867" {
		t.Errorf("expected 127.0.0.1:9867, got %s", got)
	}

	cfg = &RuntimeConfig{Bind: "0.0.0.0", Port: "8080"}
	if got := cfg.ListenAddr(); got != "0.0.0.0:8080" {
		t.Errorf("expected 0.0.0.0:8080, got %s", got)
	}
}
