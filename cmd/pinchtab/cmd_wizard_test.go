package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestRunNonInteractiveSetupDoesNotPrintToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultFileConfig()
	cfg.Server.Token = "very-secret-token-value"
	cfg.Security.AllowedDomains = []string{"localhost"}

	output := captureStdout(t, func() {
		if !runNonInteractiveSetup(&cfg, configPath, true) {
			t.Fatal("runNonInteractiveSetup() = false")
		}
	})

	if strings.Contains(output, "very-secret-token-value") {
		t.Fatalf("expected token to stay hidden, got %q", output)
	}
	if strings.Contains(output, "Token:") {
		t.Fatalf("expected setup output to omit token preview, got %q", output)
	}
}

func TestRunUpgradeNoticeDoesNotPrintToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultFileConfig()
	cfg.ConfigVersion = "0.9.0"
	cfg.Server.Token = "very-secret-token-value"

	output := captureStdout(t, func() {
		if !runUpgradeNotice(&cfg, configPath) {
			t.Fatal("runUpgradeNotice() = false")
		}
	})

	if strings.Contains(output, "very-secret-token-value") {
		t.Fatalf("expected token to stay hidden, got %q", output)
	}
	if strings.Contains(output, "Token:") {
		t.Fatalf("expected upgrade output to omit token preview, got %q", output)
	}
}

func TestApplyPostureGuardUp(t *testing.T) {
	cfg := &config.FileConfig{}
	cfg.Server.Bind = "0.0.0.0" // sentinel: Guard UP must overwrite the bind

	applyPosture(cfg, guardUpPosture)

	allows := map[string]*bool{
		"AllowEvaluate":   cfg.Security.AllowEvaluate,
		"AllowDownload":   cfg.Security.AllowDownload,
		"AllowCookies":    cfg.Security.AllowCookies,
		"AllowUpload":     cfg.Security.AllowUpload,
		"AllowMacro":      cfg.Security.AllowMacro,
		"AllowScreencast": cfg.Security.AllowScreencast,
	}
	for name, p := range allows {
		if p == nil || *p != false {
			t.Errorf("Guard UP %s = %v, want non-nil false", name, p)
		}
	}
	if !cfg.Security.IDPI.Enabled || !cfg.Security.IDPI.StrictMode ||
		!cfg.Security.IDPI.ScanContent || !cfg.Security.IDPI.WrapContent {
		t.Errorf("Guard UP IDPI = %+v, want all enabled", cfg.Security.IDPI)
	}
	got := cfg.Security.AllowedDomains
	if len(got) != 3 || got[0] != "127.0.0.1" || got[1] != "localhost" || got[2] != "::1" {
		t.Errorf("Guard UP AllowedDomains = %v, want loopback trio", got)
	}
	if cfg.Server.Bind != "127.0.0.1" {
		t.Errorf("Guard UP Bind = %q, want 127.0.0.1", cfg.Server.Bind)
	}
}

func TestApplyPostureGuardDown(t *testing.T) {
	cfg := &config.FileConfig{}
	cfg.Server.Bind = "203.0.113.5" // sentinel: Guard DOWN must leave the bind untouched

	applyPosture(cfg, guardDownPosture)

	allows := map[string]*bool{
		"AllowEvaluate":   cfg.Security.AllowEvaluate,
		"AllowDownload":   cfg.Security.AllowDownload,
		"AllowCookies":    cfg.Security.AllowCookies,
		"AllowUpload":     cfg.Security.AllowUpload,
		"AllowMacro":      cfg.Security.AllowMacro,
		"AllowScreencast": cfg.Security.AllowScreencast,
	}
	for name, p := range allows {
		if p == nil || *p != true {
			t.Errorf("Guard DOWN %s = %v, want non-nil true", name, p)
		}
	}
	if cfg.Security.IDPI.Enabled || cfg.Security.IDPI.StrictMode ||
		cfg.Security.IDPI.ScanContent || cfg.Security.IDPI.WrapContent {
		t.Errorf("Guard DOWN IDPI = %+v, want all disabled", cfg.Security.IDPI)
	}
	if len(cfg.Security.AllowedDomains) != 0 {
		t.Errorf("Guard DOWN AllowedDomains = %v, want empty", cfg.Security.AllowedDomains)
	}
	if cfg.Server.Bind != "203.0.113.5" {
		t.Errorf("Guard DOWN Bind = %q, want the sentinel left unchanged", cfg.Server.Bind)
	}
}

// The printed summary must reflect the same posture struct the config mutator
// uses, so the wizard cannot promise a posture it does not persist.
func TestPrintPostureReflectsPosture(t *testing.T) {
	up := captureStdout(t, func() { printPosture(guardUpPosture) })
	for _, want := range []string{"127.0.0.1, localhost, ::1", "disabled", "strict"} {
		if !strings.Contains(up, want) {
			t.Errorf("Guard UP summary missing %q, got:\n%s", want, up)
		}
	}
	if strings.Contains(up, "enabled") {
		t.Errorf("Guard UP summary should not advertise an enabled feature, got:\n%s", up)
	}

	down := captureStdout(t, func() { printPosture(guardDownPosture) })
	for _, want := range []string{"all", "enabled", "off"} {
		if !strings.Contains(down, want) {
			t.Errorf("Guard DOWN summary missing %q, got:\n%s", want, down)
		}
	}
	if strings.Contains(down, "strict") {
		t.Errorf("Guard DOWN summary should not advertise IDPI strict, got:\n%s", down)
	}
}
