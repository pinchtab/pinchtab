package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndSaveFileConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	// Load (should return empty config for non-existent file)
	fc, path, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if path != configPath {
		t.Errorf("path = %v, want %v", path, configPath)
	}

	// Modify
	fc.Server.Port = "8080"
	fc.InstanceDefaults.StealthLevel = "full"

	// Save
	if err := SaveFileConfig(fc, path); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	// Load again
	fc2, _, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() second time error = %v", err)
	}

	if fc2.Server.Port != "8080" {
		t.Errorf("loaded port = %v, want 8080", fc2.Server.Port)
	}
	if fc2.InstanceDefaults.StealthLevel != "full" {
		t.Errorf("loaded stealthLevel = %v, want full", fc2.InstanceDefaults.StealthLevel)
	}
}

func TestLoadAndSaveFileConfigPreservesExplicitZeroValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	fc := DefaultFileConfig()
	fc.Server.Bind = ""
	fc.Server.Token = ""
	fc.Browser.ExtensionPaths = []string{}
	fc.InstanceDefaults.UserAgent = ""
	fc.Security.IDPI.StrictMode = false
	fc.Security.IDPI.AllowedDomains = []string{}
	fc.Security.IDPI.CustomPatterns = []string{}

	if err := SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	loaded, _, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}

	if loaded.Server.Bind != "" {
		t.Errorf("loaded bind = %q, want empty string", loaded.Server.Bind)
	}
	if loaded.Security.IDPI.StrictMode {
		t.Errorf("loaded strictMode = %v, want false", loaded.Security.IDPI.StrictMode)
	}
	if len(loaded.Security.IDPI.AllowedDomains) != 0 {
		t.Errorf("loaded allowedDomains = %v, want empty list", loaded.Security.IDPI.AllowedDomains)
	}
	if len(loaded.Browser.ExtensionPaths) != 0 {
		t.Errorf("loaded extensionPaths = %v, want empty list", loaded.Browser.ExtensionPaths)
	}
}
