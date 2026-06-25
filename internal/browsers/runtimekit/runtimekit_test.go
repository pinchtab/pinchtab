package runtimekit

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/builtin"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestRuntimeProxyConfig_NilConfig(t *testing.T) {
	got := RuntimeProxyConfig(nil)
	if got.Server != "" || got.Username != "" || got.Password != "" || got.BypassList != nil || got.Geo != nil {
		t.Fatalf("RuntimeProxyConfig(nil) = %+v, want zero value", got)
	}
}

func TestRuntimeProxyConfig_MapsFields(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Proxy: config.BrowserProxyConfig{
			Server:     "http://proxy.example:8080",
			Username:   "user",
			Password:   "pw",
			BypassList: []string{"*.internal", "localhost"},
			Geo: &config.BrowserProxyGeoConfig{
				Timezone:   "Europe/Rome",
				Locale:     "it-IT",
				WebRTCIP:   "203.0.113.7",
				CountryISO: "IT",
			},
		},
	}
	got := RuntimeProxyConfig(cfg)
	if got.Server != "http://proxy.example:8080" || got.Username != "user" || got.Password != "pw" {
		t.Fatalf("proxy fields not mapped: %+v", got)
	}
	if len(got.BypassList) != 2 || got.BypassList[0] != "*.internal" {
		t.Fatalf("bypass list not mapped: %v", got.BypassList)
	}
	if got.Geo == nil || got.Geo.Timezone != "Europe/Rome" || got.Geo.Locale != "it-IT" ||
		got.Geo.WebRTCIP != "203.0.113.7" || got.Geo.CountryISO != "IT" {
		t.Fatalf("geo not mapped: %+v", got.Geo)
	}
}

func TestRuntimeProxyConfig_CopiesBypassList(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Proxy: config.BrowserProxyConfig{
			Server:     "http://proxy.example:8080",
			BypassList: []string{"a.example"},
		},
	}
	got := RuntimeProxyConfig(cfg)
	cfg.Proxy.BypassList[0] = "mutated.example"
	if got.BypassList[0] != "a.example" {
		t.Fatalf("BypassList aliases the input slice; want an independent copy")
	}
}

func TestRuntimeProxyConfig_NilGeoStaysNil(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Proxy: config.BrowserProxyConfig{Server: "http://proxy.example:8080"},
	}
	if got := RuntimeProxyConfig(cfg); got.Geo != nil {
		t.Fatalf("Geo = %+v, want nil when source geo is nil", got.Geo)
	}
}

func TestLaunchConfigFromRuntime_MapsFields(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: "chrome",
		ProfileDir:     "/tmp/profile",
		Headless:       true,
		Timezone:       "UTC",
		ExtensionPaths: []string{"/ext/a"},
		NoRestore:      true,
		UserAgent:      "ua-test",
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed: "seed",
			Platform:        "macos",
			Locale:          "en-US",
			Timezone:        "Europe/Rome",
			WebRTCIP:        "203.0.113.7",
			FontsDir:        "/fonts",
			StorageQuotaMB:  64,
		},
	}
	got := LaunchConfigFromRuntime(cfg, "/bin/chrome", 9222, true)
	if got.Binary != "/bin/chrome" || got.DebugPort != 9222 || !got.NoSandbox {
		t.Fatalf("binary/port/sandbox not mapped: %+v", got)
	}
	if got.ProfileDir != "/tmp/profile" || !got.Headless || got.Timezone != "UTC" ||
		got.NoRestore != true || got.UserAgent != "ua-test" {
		t.Fatalf("runtime fields not mapped: %+v", got)
	}
	if len(got.ExtensionPaths) != 1 || got.ExtensionPaths[0] != "/ext/a" {
		t.Fatalf("extension paths not mapped: %v", got.ExtensionPaths)
	}
	if got.Cloak.FingerprintSeed != "seed" || got.Cloak.Platform != "macos" ||
		got.Cloak.FontsDir != "/fonts" || got.Cloak.StorageQuotaMB != 64 {
		t.Fatalf("cloak fingerprint not mapped: %+v", got.Cloak)
	}
	if got.Mode != browsers.LaunchModeChrome {
		t.Fatalf("Mode = %v, want LaunchModeChrome for chrome", got.Mode)
	}
}

func TestLaunchConfigFromRuntime_GhostChromeUsesAutoMode(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: "ghost-chrome"}
	if got := LaunchConfigFromRuntime(cfg, "", 0, false); got.Mode != browsers.LaunchModeAuto {
		t.Fatalf("Mode = %v, want LaunchModeAuto for ghost-chrome", got.Mode)
	}
}

func TestChromeNeedsNoSandbox(t *testing.T) {
	cases := []struct {
		name        string
		goos        string
		euid        int
		inContainer bool
		want        bool
	}{
		{name: "darwin never needs it", goos: "darwin", euid: 0, inContainer: true, want: false},
		{name: "windows never needs it", goos: "windows", euid: 0, inContainer: true, want: false},
		{name: "linux root", goos: "linux", euid: 0, inContainer: false, want: true},
		{name: "linux non-root in container", goos: "linux", euid: 1000, inContainer: true, want: true},
		{name: "linux non-root no container", goos: "linux", euid: 1000, inContainer: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChromeNeedsNoSandbox(tc.goos, tc.euid, tc.inContainer); got != tc.want {
				t.Fatalf("ChromeNeedsNoSandbox(%q, %d, %v) = %v, want %v", tc.goos, tc.euid, tc.inContainer, got, tc.want)
			}
		})
	}
}

func TestResolveProviderLaunchPlan_Chrome(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: "chrome"}
	launchCfg := LaunchConfigFromRuntime(cfg, "  /bin/chrome  ", 0, false)
	plan, err := ResolveProviderLaunchPlan(cfg, launchCfg)
	if err != nil {
		t.Fatalf("ResolveProviderLaunchPlan returned %v", err)
	}
	if plan.Browser == nil {
		t.Fatal("plan.Browser is nil; expected the registered chrome provider")
	}
	if plan.Binary != "/bin/chrome" {
		t.Fatalf("Binary = %q, want trimmed %q", plan.Binary, "/bin/chrome")
	}
	if len(plan.Args) == 0 {
		t.Fatal("plan.Args is empty; expected chrome base flags")
	}
	found := false
	for _, arg := range plan.Args {
		if arg == "--disable-background-networking" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("plan.Args missing a known chrome base flag: %v", plan.Args)
	}
}

func TestFindBrowserBinary_UnknownProvider(t *testing.T) {
	if got := FindBrowserBinary("no-such-browser"); got != "" {
		t.Fatalf("FindBrowserBinary(unknown) = %q, want empty", got)
	}
}

func TestBaseFlagArgs_Chrome(t *testing.T) {
	args := BaseFlagArgs("chrome", false)
	if len(args) == 0 {
		t.Fatal("BaseFlagArgs(chrome, false) is empty")
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			t.Fatalf("unexpected non-flag arg %q in %v", arg, args)
		}
	}
}

func TestCloakBrowserFlagArgs_NilAndNonCloak(t *testing.T) {
	if got := CloakBrowserFlagArgs(nil); got != nil {
		t.Fatalf("CloakBrowserFlagArgs(nil) = %v, want nil", got)
	}
	cfg := &config.RuntimeConfig{DefaultBrowser: "chrome"}
	if got := CloakBrowserFlagArgs(cfg); got != nil {
		t.Fatalf("CloakBrowserFlagArgs(chrome cfg) = %v, want nil", got)
	}
}

func TestCloakBrowserFlagArgs_OnlyFingerprintFlagsSurvive(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserCloak,
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed: "seed-1",
			Timezone:        "Europe/Rome",
		},
	}
	args := CloakBrowserFlagArgs(cfg)
	if len(args) == 0 {
		t.Fatal("expected fingerprint flags for cloak config, got none")
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--fingerprint") {
			t.Fatalf("non-fingerprint flag %q leaked into cloak flag args: %v", arg, args)
		}
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "seed-1") || !strings.Contains(joined, "Europe/Rome") {
		t.Fatalf("configured fingerprint values missing from args: %v", args)
	}
}
