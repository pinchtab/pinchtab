package config

import (
	"os"
	"path/filepath"
	"runtime"
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
	fc.Security.AllowedDomains = []string{}
	fc.Security.IDPI.CustomPatterns = []string{}
	fc.Security.IDPI.ShieldThreshold = 30

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
	if len(loaded.Security.AllowedDomains) != 0 {
		t.Errorf("loaded security.allowedDomains = %v, want empty list", loaded.Security.AllowedDomains)
	}
	if loaded.Security.IDPI.ShieldThreshold != 30 {
		t.Errorf("loaded shieldThreshold = %d, want 30", loaded.Security.IDPI.ShieldThreshold)
	}
	if len(loaded.Browser.ExtensionPaths) != 0 {
		t.Errorf("loaded extensionPaths = %v, want empty list", loaded.Browser.ExtensionPaths)
	}
}

func TestLoadFileConfig_PromotesLegacyIDPIAllowedDomains(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	_ = os.Setenv("PINCHTAB_CONFIG", configPath)
	defer func() { _ = os.Unsetenv("PINCHTAB_CONFIG") }()

	data := []byte(`{
  "security": {
    "idpi": {
      "enabled": true,
      "allowedDomains": ["fixtures", "*.example.com"]
    }
  }
}`)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, _, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if got := loaded.Security.AllowedDomains; len(got) != 2 || got[0] != "fixtures" || got[1] != "*.example.com" {
		t.Fatalf("security.allowedDomains = %v, want promoted legacy values", got)
	}
}

func TestSaveFileConfigSetsOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only file mode semantics")
	}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sub", "config.json")

	fc := DefaultFileConfig()
	if err := SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	if fi, err := os.Stat(configPath); err != nil {
		t.Fatalf("stat config: %v", err)
	} else if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("config.json mode = %o, want 0600", got)
	}
	if fi, err := os.Stat(filepath.Dir(configPath)); err != nil {
		t.Fatalf("stat dir: %v", err)
	} else if got := fi.Mode().Perm(); got != 0700 {
		t.Errorf("config dir mode = %o, want 0700", got)
	}
}

func TestSaveFileConfigTightensExistingLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only file mode semantics")
	}
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "loose")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	fc := DefaultFileConfig()
	if err := SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	if fi, err := os.Stat(configPath); err != nil {
		t.Fatal(err)
	} else if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("config.json mode = %o, want 0600 after tightening", got)
	}
	if fi, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	} else if got := fi.Mode().Perm(); got != 0700 {
		t.Errorf("config dir mode = %o, want 0700 after tightening", got)
	}
}

func TestLoadFileConfigTightensExistingLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only file mode semantics")
	}
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "loose")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PINCHTAB_CONFIG", configPath)
	if _, _, err := LoadFileConfig(); err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}

	if fi, err := os.Stat(configPath); err != nil {
		t.Fatal(err)
	} else if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("config.json mode after load = %o, want 0600", got)
	}
	if fi, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	} else if got := fi.Mode().Perm(); got != 0700 {
		t.Errorf("config dir mode after load = %o, want 0700", got)
	}
}
