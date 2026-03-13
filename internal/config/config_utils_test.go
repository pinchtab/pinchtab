package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.8.0", "0.8.0", 0},
		{"0.7.0", "0.8.0", -1},
		{"0.8.0", "0.7.0", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.8.1", "0.8.0", 1},
		{"0.8.0", "0.8.1", -1},
		{"1.0.0", "1.0.0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := CompareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNeedsWizard(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{"empty version", "", true},
		{"old version", "0.7.0", true},
		{"current version", CurrentConfigVersion, false},
		{"future version", "1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &FileConfig{ConfigVersion: tt.version}
			if got := NeedsWizard(cfg); got != tt.want {
				t.Errorf("NeedsWizard(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestIsFirstRun(t *testing.T) {
	if !IsFirstRun(&FileConfig{}) {
		t.Error("expected IsFirstRun for empty config")
	}
	if IsFirstRun(&FileConfig{ConfigVersion: "0.8.0"}) {
		t.Error("expected not IsFirstRun for versioned config")
	}
}

// TestUserConfigDirLegacyConfigFilePriority tests that when legacy config FILE exists
// but the XDG config directory already exists (without config.json), the legacy path
// is still used. This exercises the full userConfigDir path resolution for issue #224.
func TestUserConfigDirLegacyConfigFilePriority(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific XDG path test")
	}

	tmpHome, err := os.MkdirTemp("", "pinchtab-home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpHome) }()

	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))

	legacyDir := filepath.Join(tmpHome, ".pinchtab")
	newDir := filepath.Join(tmpHome, ".config", "pinchtab")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("Failed to create legacy dir: %v", err)
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("Failed to create new dir: %v", err)
	}

	legacyConfig := filepath.Join(legacyDir, "config.json")
	if err := os.WriteFile(legacyConfig, []byte(`{"server":{"port":"9876"}}`), 0644); err != nil {
		t.Fatalf("Failed to create legacy config: %v", err)
	}

	got := userConfigDir()
	if got != legacyDir {
		t.Fatalf("userConfigDir() = %q, want legacy path %q", got, legacyDir)
	}
}

func TestUserConfigDirPrefersNewConfigFileWhenPresent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific XDG path test")
	}

	tmpHome, err := os.MkdirTemp("", "pinchtab-home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpHome) }()

	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))

	legacyDir := filepath.Join(tmpHome, ".pinchtab")
	newDir := filepath.Join(tmpHome, ".config", "pinchtab")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("Failed to create legacy dir: %v", err)
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("Failed to create new dir: %v", err)
	}

	for _, path := range []string{
		filepath.Join(legacyDir, "config.json"),
		filepath.Join(newDir, "config.json"),
	} {
		if err := os.WriteFile(path, []byte(`{"server":{"port":"9876"}}`), 0644); err != nil {
			t.Fatalf("Failed to create config %s: %v", path, err)
		}
	}

	got := userConfigDir()
	if got != newDir {
		t.Fatalf("userConfigDir() = %q, want new XDG path %q", got, newDir)
	}
}
