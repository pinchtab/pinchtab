package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoad_EngineMigratesToDefaultBrowser(t *testing.T) {
	tests := []struct {
		name               string
		config             string
		wantEngine         string
		wantDefaultBrowser string
	}{
		{
			name:               "engine chrome maps to chrome",
			config:             `{"server":{"engine":"chrome"}}`,
			wantEngine:         "chrome",
			wantDefaultBrowser: BrowserChrome,
		},
		{
			name:               "engine lite maps to ghost-chrome",
			config:             `{"server":{"engine":"lite"}}`,
			wantEngine:         "lite",
			wantDefaultBrowser: BrowserGhostChrome,
		},
		{
			name:               "engine auto maps to ghost-chrome",
			config:             `{"server":{"engine":"auto"}}`,
			wantEngine:         "auto",
			wantDefaultBrowser: BrowserGhostChrome,
		},
		{
			name:               "browsers.default wins over engine",
			config:             `{"server":{"engine":"lite"},"browsers":{"default":"cloak"}}`,
			wantEngine:         "lite",
			wantDefaultBrowser: BrowserCloak,
		},
		{
			name:               "browser.provider ignored (no longer supported)",
			config:             `{"browser":{"provider":"cloak"}}`,
			wantEngine:         "chrome",
			wantDefaultBrowser: "chrome",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(configPath, []byte(tt.config), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PINCHTAB_CONFIG", configPath)
			cfg := Load()
			if cfg.Engine != tt.wantEngine {
				t.Errorf("Engine = %q, want %q", cfg.Engine, tt.wantEngine)
			}
			if cfg.DefaultBrowser != tt.wantDefaultBrowser {
				t.Errorf("DefaultBrowser = %q, want %q", cfg.DefaultBrowser, tt.wantDefaultBrowser)
			}
		})
	}
}

func TestValidate_BrowserProviderTriggersError(t *testing.T) {
	fc := &FileConfig{
		Browser: BrowserConfig{Provider: "cloak"},
	}
	errs := ValidateFileConfig(fc)
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "browser.provider is no longer supported") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected validation error for browser.provider, got: %v", errs)
	}
}
