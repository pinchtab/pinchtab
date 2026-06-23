package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestCloakChildConfigRoundTrip(t *testing.T) {
	// Isolate from the developer's real config: the first Load must read
	// defaults, not whatever ~/.pinchtab or PINCHTAB_CONFIG points at.
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(t.TempDir(), "absent.json"))
	cfg := config.Load()
	cfg.DefaultBrowser = "cloak"
	cfg.Cloak.DisableDefaultStealthArgs = true
	cfg.Cloak.FingerprintSeed = "42069"

	// Step 2: Convert to FileConfig (what buildChildFileConfig does)
	fc := config.FileConfigFromRuntime(cfg)
	t.Logf("Step 2 - FileConfig.Browsers.Default = %q", fc.Browsers.Default)
	if fc.Browsers.Default != "cloak" {
		t.Fatalf("FileConfigFromRuntime lost browsers.default: got %q", fc.Browsers.Default)
	}

	// Step 3: Marshal to JSON (what writeChildConfig does)
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if b, ok := raw["browsers"]; ok {
		var browsersRaw map[string]json.RawMessage
		if err := json.Unmarshal(b, &browsersRaw); err != nil {
			t.Fatal(err)
		}
		if d, ok := browsersRaw["default"]; ok {
			t.Logf("Step 3 - JSON browsers.default = %s", string(d))
		} else {
			t.Fatal("Step 3 - JSON missing browsers.default key")
		}
	} else {
		t.Fatal("Step 3 - JSON missing browsers key entirely")
	}

	// Step 4: Write to a temp file (what writeChildConfig does)
	tmp := t.TempDir()
	childConfigPath := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(childConfigPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Step 5: Load via config.Load() with PINCHTAB_CONFIG pointing to child config
	// (what the bridge subprocess does)
	t.Setenv("PINCHTAB_CONFIG", childConfigPath)
	t.Setenv("PINCHTAB_TOKEN", "test-token")

	childCfg := config.Load()
	t.Logf("Step 5 - childCfg.DefaultBrowser = %q", childCfg.DefaultBrowser)
	if childCfg.DefaultBrowser != "cloak" {
		t.Fatalf("config.Load() lost browsers.default: got %q, want %q", childCfg.DefaultBrowser, "cloak")
	}
	if !childCfg.Cloak.DisableDefaultStealthArgs {
		t.Error("config.Load() lost cloak.disableDefaultStealthArgs")
	}
	if childCfg.Cloak.FingerprintSeed != "42069" {
		t.Errorf("config.Load() lost cloak.fingerprintSeed: got %q, want %q", childCfg.Cloak.FingerprintSeed, "42069")
	}
}

func TestCloakTargetResolution(t *testing.T) {
	// Isolate from the developer's real config: the first Load must read
	// defaults, not whatever ~/.pinchtab or PINCHTAB_CONFIG points at.
	t.Setenv("PINCHTAB_CONFIG", filepath.Join(t.TempDir(), "absent.json"))
	// Simulate what happens when a cloak E2E config is loaded:
	// browsers.default = "cloak" but browser.provider is NOT set.
	// The legacy migration must use browsers.default as the provider.
	cfg := config.Load()
	cfg.DefaultBrowser = "cloak"
	cfg.Cloak.DisableDefaultStealthArgs = true
	cfg.Cloak.FingerprintSeed = "42069"
	cfg.BrowserBinary = "/opt/cloakbrowser/chrome"

	// The FileConfigFromRuntime → Load round-trip produces targets via migration.
	fc := config.FileConfigFromRuntime(cfg)

	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	childConfigPath := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(childConfigPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PINCHTAB_CONFIG", childConfigPath)
	t.Setenv("PINCHTAB_TOKEN", "test-token")

	reloaded := config.Load()
	t.Logf("reloaded.DefaultBrowser = %q", reloaded.DefaultBrowser)
	t.Logf("reloaded.Targets = %v", reloaded.Targets)

	// The key test: if targets were synthesized, resolving the default
	// target must preserve the cloak provider.
	if len(reloaded.Targets) > 0 {
		resolved, err := config.ResolveDefaultBrowserTarget(reloaded)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("resolved.Provider = %q, resolved.Config.DefaultBrowser = %q",
			resolved.Provider, resolved.Config.DefaultBrowser)
		if resolved.Config.DefaultBrowser != "cloak" {
			t.Errorf("resolved target override DefaultBrowser: got %q, want %q",
				resolved.Config.DefaultBrowser, "cloak")
		}
	}
}
