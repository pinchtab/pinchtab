package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestApplyGuardsDownPreset(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	fc.Server.Token = "guarded-token"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	cfg, gotPath, changed, err := ApplyGuardsDownPreset()
	if err != nil {
		t.Fatalf("ApplyGuardsDownPreset() error = %v", err)
	}
	if !changed {
		t.Fatal("expected guards down preset to change config")
	}
	if gotPath != configPath {
		t.Fatalf("config path = %q, want %q", gotPath, configPath)
	}

	if cfg.Bind != "127.0.0.1" {
		t.Fatalf("Bind = %q, want 127.0.0.1", cfg.Bind)
	}
	if cfg.Token != "guarded-token" {
		t.Fatalf("Token = %q, want existing token to remain", cfg.Token)
	}
	if !cfg.AllowEvaluate || !cfg.AllowMacro || !cfg.AllowScreencast || !cfg.AllowDownload || !cfg.AllowUpload {
		t.Fatalf("expected sensitive endpoints enabled, got %+v", cfg)
	}
	if !cfg.AttachEnabled {
		t.Fatal("expected attach endpoint enabled")
	}
	if got := strings.Join(cfg.AttachAllowHosts, ","); got != "127.0.0.1,localhost,::1" {
		t.Fatalf("AttachAllowHosts = %q", got)
	}
	if got := strings.Join(cfg.AttachAllowSchemes, ","); got != "ws,wss" {
		t.Fatalf("AttachAllowSchemes = %q", got)
	}
	if cfg.IDPI.Enabled || cfg.IDPI.StrictMode || cfg.IDPI.ScanContent || cfg.IDPI.WrapContent {
		t.Fatalf("expected IDPI protections disabled, got %+v", cfg.IDPI)
	}
}
