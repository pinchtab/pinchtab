package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
)

func TestValidateBrowsersBlock_EmptyBlock(t *testing.T) {
	fc := &FileConfig{}
	errs := ValidateFileConfig(fc)
	for _, e := range errs {
		if strings.Contains(e.Error(), "browsers.") {
			t.Fatalf("empty browsers block should produce no browsers errors, got: %v", e)
		}
	}
}

func TestValidateBrowsersBlock_ValidConfig(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Default:   "chrome",
			Available: []string{"chrome", "cloak"},
		},
	}
	errs := ValidateFileConfig(fc)
	for _, e := range errs {
		if strings.Contains(e.Error(), "browsers.") {
			t.Fatalf("valid browsers block should produce no browsers errors, got: %v", e)
		}
	}
}

func TestValidateBrowsersBlock_UnknownDefault(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Default: "firefox",
		},
	}
	errs := ValidateFileConfig(fc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "browsers.default") && strings.Contains(e.Error(), "firefox") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected browsers.default error for unknown browser, got: %v", errs)
	}
}

func TestValidateBrowsersBlock_UnknownAvailable(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Available: []string{"chrome", "safari"},
		},
	}
	errs := ValidateFileConfig(fc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "browsers.available") && strings.Contains(e.Error(), "safari") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected browsers.available error for unknown browser, got: %v", errs)
	}
}

func TestValidateBrowsersBlock_DefaultNotInAvailable(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Default:   "cloak",
			Available: []string{"chrome"},
		},
	}
	errs := ValidateFileConfig(fc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "not in browsers.available") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about default not in available, got: %v", errs)
	}
}

// browsers.config was never applied anywhere; any presence — known or unknown
// browser keys, empty or populated items — is rejected with targets guidance.
func TestValidateBrowsersBlock_ConfigBlockRejected(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]BrowserItemConfig
	}{
		{"known browser key", map[string]BrowserItemConfig{"cloak": {Binary: "/opt/cloak/chrome"}}},
		{"unknown browser key", map[string]BrowserItemConfig{"edge": {Binary: "/usr/bin/edge"}}},
		{"empty item", map[string]BrowserItemConfig{"ghost-chrome": {}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &FileConfig{
				Browsers: BrowsersConfig{Config: tc.config},
			}
			errs := ValidateFileConfig(fc)
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), "browsers.config is no longer supported") &&
					strings.Contains(e.Error(), "browser.targets") {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected browsers.config not-supported error, got: %v", errs)
			}
		})
	}
}

func TestValidateBrowsersBlock_GhostChromeDefault(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Default:   "ghost-chrome",
			Available: []string{"chrome", "ghost-chrome"},
		},
	}
	errs := ValidateFileConfig(fc)
	for _, e := range errs {
		if strings.Contains(e.Error(), "browsers.") {
			t.Fatalf("ghost-chrome as default should be valid, got: %v", e)
		}
	}
}

func TestValidateBrowsersBlock_AllPhase1Browsers(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Default:   "chrome",
			Available: []string{"chrome", "ghost-chrome", "cloak"},
		},
	}
	errs := ValidateFileConfig(fc)
	for _, e := range errs {
		if strings.Contains(e.Error(), "browsers.") {
			t.Fatalf("all Phase 1 browsers should be valid, got: %v", e)
		}
	}
}

func TestValidateBrowsersBlock_EmptyConfigMap(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Default:   "chrome",
			Available: []string{"chrome"},
			Config:    map[string]BrowserItemConfig{},
		},
	}
	errs := ValidateFileConfig(fc)
	for _, e := range errs {
		if strings.Contains(e.Error(), "browsers.") {
			t.Fatalf("empty config map should be valid, got: %v", e)
		}
	}
}

func TestBrowsersBlock_DefaultsWhenAbsent(t *testing.T) {
	clearConfigEnvVars(t)
	setCloakBrowserDiscovery(t, "")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	fc := FileConfig{
		Server: ServerConfig{Port: "9867"},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", cfgPath)

	cfg := Load()
	if cfg.DefaultBrowser != "chrome" {
		t.Errorf("DefaultBrowser = %q, want %q", cfg.DefaultBrowser, "chrome")
	}
	if len(cfg.BrowsersAvailable) != 1 || cfg.BrowsersAvailable[0] != "chrome" {
		t.Errorf("BrowsersAvailable = %v, want [chrome]", cfg.BrowsersAvailable)
	}
}

func TestBrowsersBlock_DefaultsWhenAbsentPrefersInstalledCloakBrowser(t *testing.T) {
	clearConfigEnvVars(t)
	setCloakBrowserDiscovery(t, "/opt/cloakbrowser/chrome")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	fc := FileConfig{Server: ServerConfig{Port: "9867"}}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", cfgPath)

	cfg := Load()
	if cfg.DefaultBrowser != BrowserCloak {
		t.Errorf("DefaultBrowser = %q, want %q when CloakBrowser is installed", cfg.DefaultBrowser, BrowserCloak)
	}
	if len(cfg.BrowsersAvailable) != 1 || cfg.BrowsersAvailable[0] != BrowserCloak {
		t.Errorf("BrowsersAvailable = %v, want [cloak]", cfg.BrowsersAvailable)
	}
}

func TestBrowsersBlock_ExplicitChromeWinsOverInstalledCloakBrowser(t *testing.T) {
	clearConfigEnvVars(t)
	setCloakBrowserDiscovery(t, "/opt/cloakbrowser/chrome")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	fc := FileConfig{
		Server:   ServerConfig{Port: "9867"},
		Browsers: BrowsersConfig{Default: BrowserChrome},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", cfgPath)

	cfg := Load()
	if cfg.DefaultBrowser != BrowserChrome {
		t.Errorf("DefaultBrowser = %q, want explicit %q", cfg.DefaultBrowser, BrowserChrome)
	}
}

func TestBrowsersBlock_ExplicitValues(t *testing.T) {
	clearConfigEnvVars(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	fc := FileConfig{
		Server: ServerConfig{Port: "9867"},
		Browsers: BrowsersConfig{
			Default:   "cloak",
			Available: []string{"chrome", "cloak"},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", cfgPath)

	cfg := Load()
	if cfg.DefaultBrowser != "cloak" {
		t.Errorf("DefaultBrowser = %q, want %q", cfg.DefaultBrowser, "cloak")
	}
	if len(cfg.BrowsersAvailable) != 2 {
		t.Fatalf("BrowsersAvailable = %v, want [chrome cloak]", cfg.BrowsersAvailable)
	}
	if cfg.BrowsersAvailable[0] != "chrome" || cfg.BrowsersAvailable[1] != "cloak" {
		t.Errorf("BrowsersAvailable = %v, want [chrome cloak]", cfg.BrowsersAvailable)
	}
}

func TestBrowsersBlock_DefaultOnlyImpliesAvailable(t *testing.T) {
	clearConfigEnvVars(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	fc := FileConfig{
		Server: ServerConfig{Port: "9867"},
		Browsers: BrowsersConfig{
			Default: "cloak",
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", cfgPath)

	cfg := Load()
	if cfg.DefaultBrowser != "cloak" {
		t.Errorf("DefaultBrowser = %q, want %q", cfg.DefaultBrowser, "cloak")
	}
	if len(cfg.BrowsersAvailable) != 1 || cfg.BrowsersAvailable[0] != "cloak" {
		t.Errorf("BrowsersAvailable = %v, want [cloak]", cfg.BrowsersAvailable)
	}
}

func TestBrowsersBlock_GhostChromeLoaded(t *testing.T) {
	clearConfigEnvVars(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	fc := FileConfig{
		Server: ServerConfig{Port: "9867"},
		Browsers: BrowsersConfig{
			Default:   "ghost-chrome",
			Available: []string{"chrome", "ghost-chrome"},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", cfgPath)

	cfg := Load()
	if cfg.DefaultBrowser != "ghost-chrome" {
		t.Errorf("DefaultBrowser = %q, want %q", cfg.DefaultBrowser, "ghost-chrome")
	}
	if len(cfg.BrowsersAvailable) != 2 {
		t.Fatalf("BrowsersAvailable = %v, want [chrome ghost-chrome]", cfg.BrowsersAvailable)
	}
	found := false
	for _, name := range cfg.BrowsersAvailable {
		if name == "ghost-chrome" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("BrowsersAvailable = %v, missing ghost-chrome", cfg.BrowsersAvailable)
	}
}

// M1 regression: known providers validate case-insensitively, matching
// target-provider validation semantics.
func TestValidateBrowsersBlock_CaseInsensitiveKnownBrowsers(t *testing.T) {
	fc := &FileConfig{
		Browsers: BrowsersConfig{
			Default:   "Cloak",
			Available: []string{"CHROME", "Cloak"},
		},
	}
	errs := ValidateFileConfig(fc)
	for _, e := range errs {
		if strings.Contains(e.Error(), "unknown browser") {
			t.Fatalf("mixed-case known browsers should validate, got: %v", e)
		}
	}
}
