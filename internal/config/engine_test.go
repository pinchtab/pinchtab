package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EngineFromConfig(t *testing.T) {
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	cfg := Load()
	if cfg.Engine != "chrome" {
		t.Fatalf("default engine = %q, want chrome", cfg.Engine)
	}

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"server":{"engine":"lite"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", configPath)
	cfg = Load()
	if cfg.Engine != "lite" {
		t.Fatalf("file engine = %q, want lite", cfg.Engine)
	}
}
