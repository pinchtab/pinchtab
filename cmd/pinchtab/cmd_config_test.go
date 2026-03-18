package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestRenderConfigOverview(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Port:              "9867",
		Strategy:          "simple",
		AllocationPolicy:  "fcfs",
		StealthLevel:      "light",
		TabEvictionPolicy: "close_lru",
		Token:             "very-long-token-secret",
	}
	output := renderConfigOverview(cfg, "/tmp/pinchtab/config.json", "http://localhost:9867", false)

	required := []string{
		"Config",
		"Strategy",
		"Allocation policy",
		"Stealth level",
		"Tab eviction",
		"Copy token",
		"More",
		"/tmp/pinchtab/config.json",
		"very...cret",
		"Dashboard:",
	}
	for _, needle := range required {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected config overview to contain %q\n%s", needle, output)
		}
	}
}

func TestClipboardCommands(t *testing.T) {
	commands := clipboardCommands()
	if len(commands) == 0 {
		t.Fatal("expected clipboard commands")
	}
	for _, command := range commands {
		if command.name == "" {
			t.Fatalf("clipboard command missing name: %+v", command)
		}
	}
}

func TestConfigSetAllowsDashPrefixedValue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "browser.extraFlags", "--no-sandbox --disable-gpu"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !strings.Contains(output, "Set browser.extraFlags = --no-sandbox --disable-gpu") {
		t.Fatalf("expected success output, got %q", output)
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Browser.ChromeExtraFlags != "--no-sandbox --disable-gpu" {
		t.Fatalf("ChromeExtraFlags = %q, want %q", saved.Browser.ChromeExtraFlags, "--no-sandbox --disable-gpu")
	}
}

func TestConfigShowLoadsLegacyFlatConfigWithoutToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	if err := os.WriteFile(configPath, []byte(`{
  "port": "8765",
  "headless": true,
  "maxTabs": 30
}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "show"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !strings.Contains(output, "8765") {
		t.Fatalf("expected config show output to contain legacy port, got %q", output)
	}
	if !strings.Contains(output, "Current configuration") {
		t.Fatalf("expected config show header, got %q", output)
	}
}
