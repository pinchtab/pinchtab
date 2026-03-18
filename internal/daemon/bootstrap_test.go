package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureConfigCreatesDefaultConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")
	t.Setenv("PINCHTAB_BIND", "")

	gotPath, cfg, status, err := EnsureConfig(false)
	if err != nil {
		t.Fatalf("EnsureConfig returned error: %v", err)
	}
	if status != ConfigCreated {
		t.Fatalf("status = %q, want %q", status, ConfigCreated)
	}
	if gotPath != configPath {
		t.Fatalf("config path = %q, want %q", gotPath, configPath)
	}
	if cfg.Server.Bind != "127.0.0.1" {
		t.Fatalf("bind = %q, want 127.0.0.1", cfg.Server.Bind)
	}
	if strings.TrimSpace(cfg.Server.Token) == "" {
		t.Fatal("expected generated token to be set")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"bind": "127.0.0.1"`) {
		t.Fatalf("expected config file to include bind, got %s", content)
	}
	if !strings.Contains(content, `"token": "`) {
		t.Fatalf("expected config file to include token, got %s", content)
	}
}

func TestEnsureConfigRecoversExistingSecuritySettings(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	input := `{
  "server": {
    "bind": "0.0.0.0",
    "port": "9999",
    "token": ""
  },
  "browser": {
    "binary": "/custom/chrome"
  },
  "security": {
    "allowEvaluate": true
  }
}
`
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(input), 0644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	_, cfg, status, err := EnsureConfig(false)
	if err != nil {
		t.Fatalf("EnsureConfig returned error: %v", err)
	}
	if status != ConfigRecovered {
		t.Fatalf("status = %q, want %q", status, ConfigRecovered)
	}
	if cfg.Server.Bind != "0.0.0.0" {
		t.Fatalf("bind = %q, want 0.0.0.0 (preserved)", cfg.Server.Bind)
	}
	if cfg.Server.Port != "9999" {
		t.Fatalf("port = %q, want 9999", cfg.Server.Port)
	}
	if cfg.Browser.ChromeBinary != "/custom/chrome" {
		t.Fatalf("chrome binary = %q, want /custom/chrome", cfg.Browser.ChromeBinary)
	}
	if cfg.Security.AllowEvaluate == nil || !*cfg.Security.AllowEvaluate {
		t.Fatal("expected allowEvaluate to be preserved as true")
	}
	if strings.TrimSpace(cfg.Server.Token) == "" {
		t.Fatal("expected recovery to generate a token")
	}
}
