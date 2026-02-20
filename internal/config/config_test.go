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
	defer os.Unsetenv(key)
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
	// Clear relevant env vars
	_ = os.Unsetenv("BRIDGE_PORT")
	_ = os.Unsetenv("BRIDGE_BIND")
	_ = os.Unsetenv("CDP_URL")
	_ = os.Unsetenv("BRIDGE_TOKEN")

	cfg := Load()
	if cfg.Port != "9867" {
		t.Errorf("default Port = %v, want 9867", cfg.Port)
	}
	if cfg.Bind != "127.0.0.1" {
		t.Errorf("default Bind = %v, want 127.0.0.1", cfg.Bind)
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	_ = os.Setenv("BRIDGE_PORT", "1234")
	defer os.Unsetenv("BRIDGE_PORT")

	cfg := Load()
	if cfg.Port != "1234" {
		t.Errorf("env Port = %v, want 1234", cfg.Port)
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
	_ = os.Setenv("BRIDGE_CONFIG", configPath)
	defer os.Unsetenv("BRIDGE_CONFIG")

	// Create a dummy config file
	configData := `{
		"port": "8888",
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
	if cfg.Headless != false {
		t.Errorf("file Headless = %v, want false", cfg.Headless)
	}
	if cfg.ActionTimeout != 60*time.Second {
		t.Errorf("file ActionTimeout = %v, want 60s", cfg.ActionTimeout)
	}
}
