package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all" // register browser providers for DiscoverBrowserBinary
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.2.3", "1.2.4", -1},
		{"2.0.0", "1.9.9", 1},
		{"v120.0.0", "120.0.0", 0},
		{"V120", "120.0.0", 0},
		{"146.0.7680.177", "120.0.0", 1},
		{"99.0.4844.51", "120.0.0", -1},
		{"120", "120.0.0", 0},
		{"120.1", "120.0.99", 1},
		{"1.2.3-alpha", "1.2.3", 0},
		{"1.2.3+build.5", "1.2.3", 0},
		{"1.2.3-rc1", "1.2.4", -1},
		{"120.0.7680a", "120.0.7680", 0},
		{"", "0.0.0", 0},
	}
	for _, c := range cases {
		t.Run(c.a+"_vs_"+c.b, func(t *testing.T) {
			if got := compareSemver(c.a, c.b); got != c.want {
				t.Fatalf("compareSemver(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestExtractVersionToken(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Google Chrome 146.0.7680.177", "146.0.7680.177"},
		{"Chromium 99.0.4844.51 snap", "99.0.4844.51"},
		{"weird (120.0.0)", "120.0.0"},
		{"no version here", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := extractVersionToken(c.in); got != c.want {
			t.Errorf("extractVersionToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfigFile_Found(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"configVersion":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	r := checkConfigFile(context.Background(), nil)
	if r.Status != StatusPass {
		t.Fatalf("status = %v want pass; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
	if !strings.Contains(r.Detail, path) {
		t.Errorf("detail %q should contain path %q", r.Detail, path)
	}
}

func TestConfigFile_Missing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.json")
	t.Setenv("PINCHTAB_CONFIG", missing)
	r := checkConfigFile(context.Background(), nil)
	if r.Status != StatusWarn {
		t.Fatalf("status = %v want warn; detail=%q", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "not found") {
		t.Errorf("expected 'not found' in detail, got %q", r.Detail)
	}
}

func TestConfigFile_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	r := checkConfigFile(context.Background(), nil)
	if r.Status != StatusFail {
		t.Fatalf("status = %v want fail; detail=%q", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "parse error") {
		t.Errorf("expected 'parse error' in detail, got %q", r.Detail)
	}
}

// withStubBinary installs a fake "chrome --version" script as the first
// $PATH entry for the duration of the test and returns its directory.
func withStubBinary(t *testing.T, name, versionOutput string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub binary tests require unix shell")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo '" + versionOutput + "'; fi\n"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return dir
}

// runChromePresent exercises the provider-supplied chrome_present check
// through the doctor registry, matching the wiring in Registry().
func runChromePresent(t *testing.T) CheckResult {
	t.Helper()
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserChrome}
	results := Run(context.Background(), cfg, "chrome_present")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for chrome_present filter, got %d", len(results))
	}
	return results[0]
}

func TestChromePresent_PassWithRecentVersion(t *testing.T) {
	withStubBinary(t, "google-chrome", "Google Chrome 146.0.7680.177")
	r := runChromePresent(t)
	if r.Status != StatusPass {
		t.Fatalf("status = %v want pass; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
	if !strings.Contains(r.Detail, "146.0.7680.177") {
		t.Errorf("expected version in detail, got %q", r.Detail)
	}
}

func TestChromePresent_WarnOnOldVersion(t *testing.T) {
	withStubBinary(t, "google-chrome", "Google Chrome 99.0.4844.51")
	r := runChromePresent(t)
	if r.Status != StatusWarn {
		t.Fatalf("status = %v want warn; detail=%q", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "<") {
		t.Errorf("expected '< required' marker in detail, got %q", r.Detail)
	}
}

func TestChromePresent_FailWhenAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	// The chrome provider uses runtime.GOOS for common-path discovery.
	// If the real Chrome is installed at a common path for the current OS
	// (e.g. /Applications/… on macOS) it will still be found even though
	// PATH is empty. Skip when that is the case — the FailWhenAbsent
	// scenario only applies on systems that truly lack a Chrome install.
	r := runChromePresent(t)
	if r.Status == StatusPass || r.Status == StatusWarn {
		t.Skipf("real Chrome found on this machine; cannot simulate absence (detail=%q)", r.Detail)
	}

	if r.Status != StatusFail {
		t.Fatalf("status = %v want fail; detail=%q", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "no chrome") {
		t.Errorf("expected 'no chrome' detail, got %q", r.Detail)
	}
}

func TestChromePresent_SkipOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("chrome provider skip-on-windows requires actual windows")
	}
	r := runChromePresent(t)
	if r.Status != StatusSkip {
		t.Fatalf("status = %v want skip", r.Status)
	}
}

// runCloakPresent exercises the provider-supplied cloakbrowser_present check
// through the doctor registry, matching the wiring in Registry().
func runCloakPresent(t *testing.T) CheckResult {
	t.Helper()
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserCloak}
	results := Run(context.Background(), cfg, "cloakbrowser_present")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for cloakbrowser_present filter, got %d", len(results))
	}
	return results[0]
}

func TestCloakBrowserPresent_NotInRegistryWhenUnconfigured(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserChrome}
	if KnownCheck(cfg, "cloakbrowser_present") {
		t.Fatal("cloakbrowser_present should not appear in registry when provider is chrome")
	}
}

func TestCloakBrowserPresent_FailWhenAbsentAndConfigured(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	r := runCloakPresent(t)
	if r.Status != StatusFail {
		t.Fatalf("status = %v want fail; detail=%q", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "cloakbrowser not found") {
		t.Errorf("expected 'cloakbrowser not found' in detail, got %q", r.Detail)
	}
}

func TestCloakBrowserPresent_PassWithStub(t *testing.T) {
	withStubBinary(t, "cloakbrowser", "CloakBrowser 130.0.6723.91")
	t.Setenv("HOME", t.TempDir())
	r := runCloakPresent(t)
	if r.Status != StatusPass {
		t.Fatalf("status = %v want pass; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
}

// TestCloakCDPReachable_ResolvesTargetBinaryThroughRegistry proves the registry
// `--check` path builds its env via doctorEnvForBrowser (the per-target cloak
// binary) rather than the global buildDoctorEnv.
//
// The cdp_reachable check reads env.Binary: with no binary it SKIPs ("no
// browser.binary set"); with a binary it attempts a launch. We configure a
// cloak target whose binary is a DISTINCT path from the (empty) global chrome
// binary. If the registry used the global env, env.Binary would be empty and
// the check would SKIP. Instead it resolves the target binary and attempts to
// launch it — the stub exits without a DevTools banner, so the check FAILs.
// A Fail (not Skip) therefore proves the target binary was resolved.
func TestCloakCDPReachable_ResolvesTargetBinaryThroughRegistry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub binary tests require unix shell")
	}
	// Distinct stub binary that exits immediately without emitting a DevTools
	// banner. Lives under its own temp dir — NOT on $PATH and NOT the global
	// chrome binary — so only target resolution can find it.
	dir := t.TempDir()
	stub := filepath.Join(dir, "cloak-stub")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{
		// Global chrome binary deliberately absent: if the registry used the
		// global env, cdp_reachable would skip on the empty binary.
		BrowserBinary: "",
		Targets: config.BrowserTargetsConfig{
			"cloak-1": {Provider: config.BrowserCloak, Binary: stub},
		},
	}

	results := Run(context.Background(), cfg, "cdp_reachable")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for cdp_reachable filter, got %d", len(results))
	}
	r := results[0]

	// A Skip means the env carried an empty binary — i.e. the global env, not
	// the target env. The whole point of this test is that this does NOT happen.
	if r.Status == StatusSkip {
		t.Fatalf("cdp_reachable skipped (detail=%q): registry used the global env, "+
			"not doctorEnvForBrowser's target binary", r.Detail)
	}
	if strings.Contains(r.Detail, "no browser.binary set") {
		t.Fatalf("cdp_reachable did not see the target binary (detail=%q)", r.Detail)
	}
	// The stub launches but exits without a DevTools banner, so the launch
	// attempt fails — confirming the registry resolved and launched the TARGET
	// binary (a launch could only be attempted with a non-empty target binary).
	if r.Status != StatusFail {
		t.Fatalf("status = %v want fail; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
}
