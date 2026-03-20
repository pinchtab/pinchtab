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
	cfg.Security.IDPI.AllowedDomains = []string{"localhost"}

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
