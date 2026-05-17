package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestChromePresent_PassWithRecentVersion(t *testing.T) {
	withStubBinary(t, "google-chrome", "Google Chrome 146.0.7680.177")
	r := checkChromePresent(context.Background(), nil)
	if r.Status != StatusPass {
		t.Fatalf("status = %v want pass; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
	if !strings.Contains(r.Detail, "146.0.7680.177") {
		t.Errorf("expected version in detail, got %q", r.Detail)
	}
}

func TestChromePresent_WarnOnOldVersion(t *testing.T) {
	withStubBinary(t, "google-chrome", "Google Chrome 99.0.4844.51")
	r := checkChromePresent(context.Background(), nil)
	if r.Status != StatusWarn {
		t.Fatalf("status = %v want warn; detail=%q", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "<") {
		t.Errorf("expected '< required' marker in detail, got %q", r.Detail)
	}
}

func TestChromePresent_FailWhenAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	// Force shared binary discovery to walk an OS arm with no fallback hits.
	prev := HostOS
	defer func() { HostOS = prev }()
	HostOS = func() string { return "linux" }
	r := checkChromePresent(context.Background(), nil)
	if r.Status != StatusFail {
		t.Fatalf("status = %v want fail; detail=%q", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "no chrome") {
		t.Errorf("expected 'no chrome' detail, got %q", r.Detail)
	}
}

func TestChromePresent_SkipOnWindows(t *testing.T) {
	prev := HostOS
	defer func() { HostOS = prev }()
	HostOS = func() string { return "windows" }
	r := checkChromePresent(context.Background(), nil)
	if r.Status != StatusSkip {
		t.Fatalf("status = %v want skip", r.Status)
	}
}

func TestCloakBrowserPresent_SkipWhenAbsentAndUnconfigured(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	prev := HostOS
	defer func() { HostOS = prev }()
	HostOS = func() string { return "linux" }
	cfg := &config.RuntimeConfig{BrowserProvider: config.BrowserProviderChrome}
	r := checkCloakBrowserPresent(context.Background(), cfg)
	if r.Status != StatusSkip {
		t.Fatalf("status = %v want skip; detail=%q", r.Status, r.Detail)
	}
}

func TestCloakBrowserPresent_FailWhenAbsentAndConfigured(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	prev := HostOS
	defer func() { HostOS = prev }()
	HostOS = func() string { return "linux" }
	cfg := &config.RuntimeConfig{BrowserProvider: config.BrowserProviderCloak}
	r := checkCloakBrowserPresent(context.Background(), cfg)
	if r.Status != StatusFail {
		t.Fatalf("status = %v want fail; detail=%q", r.Status, r.Detail)
	}
}

func TestCloakBrowserPresent_FailWhenTargetConfiguredCloak(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	prev := HostOS
	defer func() { HostOS = prev }()
	HostOS = func() string { return "linux" }
	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderChrome,
		Targets: config.BrowserTargetsConfig{
			"cloak-eu": config.BrowserTargetConfig{Provider: config.BrowserProviderCloak},
		},
	}
	r := checkCloakBrowserPresent(context.Background(), cfg)
	if r.Status != StatusFail {
		t.Fatalf("status = %v want fail; detail=%q", r.Status, r.Detail)
	}
}

func TestCloakBrowserPresent_PassWithStub(t *testing.T) {
	withStubBinary(t, "cloakbrowser", "CloakBrowser 130.0.6723.91")
	t.Setenv("HOME", t.TempDir())
	cfg := &config.RuntimeConfig{BrowserProvider: config.BrowserProviderCloak}
	r := checkCloakBrowserPresent(context.Background(), cfg)
	if r.Status != StatusPass {
		t.Fatalf("status = %v want pass; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
}
