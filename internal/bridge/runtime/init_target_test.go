package runtime

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	"github.com/pinchtab/pinchtab/internal/config"
)

func withoutBrowserDiscovery(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
}

func targetConfigWithCloakBinary(t *testing.T, binary string) *config.RuntimeConfig {
	t.Helper()
	return &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		DefaultTarget:  "cloak-primary",
		Targets: config.BrowserTargetsConfig{
			"chrome-alt": {Provider: config.BrowserChrome, Binary: "/nonexistent/chrome"},
			"cloak-primary": {
				Provider:   config.BrowserCloak,
				Binary:     binary,
				ExtraFlags: "--pinchtab-target-flag",
				Cloak:      config.CloakBrowserConfig{FingerprintSeed: "target-seed"},
				Proxy:      config.BrowserProxyConfig{Server: "http://127.0.0.1:8899"},
			},
		},
	}
}

func TestEffectiveLaunchTargetPrefersTargetBinaryOverDiscovery(t *testing.T) {
	withoutBrowserDiscovery(t)
	binary := filepath.Join(t.TempDir(), "cloak-binary")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	launchCfg, effective := effectiveLaunchTarget(targetConfigWithCloakBinary(t, binary))

	if effective.Binary != binary {
		t.Fatalf("effective binary = %q, want %q", effective.Binary, binary)
	}
	if effective.ID != config.BrowserCloak {
		t.Fatalf("effective provider = %q, want %q", effective.ID, config.BrowserCloak)
	}
	if launchCfg.DefaultBrowser != config.BrowserCloak {
		t.Fatalf("launch DefaultBrowser = %q, want %q", launchCfg.DefaultBrowser, config.BrowserCloak)
	}
	if launchCfg.BrowserBinary != binary {
		t.Fatalf("launch BrowserBinary = %q, want %q", launchCfg.BrowserBinary, binary)
	}
}

func TestEffectiveLaunchTargetFallsBackToDiscoveryWithoutTargets(t *testing.T) {
	withoutBrowserDiscovery(t)
	dir := t.TempDir()
	// The stub has to be something the host agrees is executable, or discovery
	// correctly refuses it and there is nothing left to fall back to. On Windows
	// that means an extension PATHEXT names: exec.LookPath will not return a bare
	// "cloakbrowser" there, and the mode bits below carry no meaning.
	discovered := filepath.Join(dir, "cloakbrowser"+exeSuffix)
	if err := os.WriteFile(discovered, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PATH", dir)

	_, effective := effectiveLaunchTarget(&config.RuntimeConfig{DefaultBrowser: config.BrowserCloak})

	if effective.Binary != discovered {
		t.Fatalf("effective binary = %q, want discovered %q", effective.Binary, discovered)
	}
}

func TestBuildBrowserArgsCarriesTargetLaunchSettings(t *testing.T) {
	withoutBrowserDiscovery(t)
	cfg := targetConfigWithCloakBinary(t, filepath.Join(t.TempDir(), "cloak-binary"))

	args := BuildBrowserArgs(cfg, 9222)

	for _, want := range []string{
		"--fingerprint=target-seed",
		"--pinchtab-target-flag",
		"--proxy-server=http://127.0.0.1:8899",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing target-derived arg %q in %v", want, args)
		}
	}
}

func TestMissingBrowserBinaryErrorNamesTargetKey(t *testing.T) {
	withoutBrowserDiscovery(t)
	launchCfg, _ := effectiveLaunchTarget(targetConfigWithCloakBinary(t, ""))

	err := missingBrowserBinaryError(launchCfg)

	for _, want := range []string{`"cloak-primary"`, "browser.targets.cloak-primary.binary"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "set browser.binary") {
		t.Fatalf("error still points at the legacy top-level key: %q", err)
	}
}
