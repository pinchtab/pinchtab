package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func TestBrowserNeedsNoSandbox(t *testing.T) {
	origGOOS := runtimeGOOS
	origGeteuid := osGeteuid
	origMarker := containerMarkerPath
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osGeteuid = origGeteuid
		containerMarkerPath = origMarker
	})

	runtimeGOOS = "linux"
	osGeteuid = func() int { return 1000 }
	containerMarkerPath = t.TempDir() + "/missing-dockerenv"

	if launchNeedsNoSandbox() {
		t.Fatal("expected no-sandbox compatibility to be disabled without root or container marker")
	}

	osGeteuid = func() int { return 0 }
	if !launchNeedsNoSandbox() {
		t.Fatal("expected root user to enable no-sandbox compatibility")
	}
	osGeteuid = func() int { return 1000 }

	containerMarkerPath = t.TempDir() + "/.dockerenv"
	if err := os.WriteFile(containerMarkerPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if !launchNeedsNoSandbox() {
		t.Fatal("expected container marker to enable no-sandbox compatibility")
	}
}

func TestShouldRetryBrowserStartupWithDirectLaunch(t *testing.T) {
	canceledParent, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name      string
		parentCtx context.Context
		err       error
		want      bool
	}{
		{
			name:      "startup timeout retries",
			parentCtx: context.Background(),
			err:       context.DeadlineExceeded,
			want:      true,
		},
		{
			name:      "allocator context canceled retries",
			parentCtx: context.Background(),
			err:       context.Canceled,
			want:      true,
		},
		{
			name:      "wrapped context canceled retries",
			parentCtx: context.Background(),
			err:       fmt.Errorf("failed to start: %w", context.Canceled),
			want:      true,
		},
		{
			name:      "string matched context canceled retries",
			parentCtx: context.Background(),
			err:       errors.New("failed to connect to chrome: context canceled"),
			want:      true,
		},
		{
			name:      "parent cancellation does not retry",
			parentCtx: canceledParent,
			err:       context.Canceled,
			want:      false,
		},
		{
			name:      "other errors do not retry",
			parentCtx: context.Background(),
			err:       errors.New("exec format error"),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryBrowserStartupWithDirectLaunch(tt.parentCtx, tt.err); got != tt.want {
				t.Fatalf("shouldRetryBrowserStartupWithDirectLaunch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildBrowserArgs_CloakProviderUsesNativeFingerprintFlags(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserCloak,
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed:           "42069",
			Platform:                  "windows",
			Locale:                    "en-GB",
			Timezone:                  "Europe/London",
			WebRTCIP:                  "auto",
			FontsDir:                  "/opt/fonts",
			StorageQuotaMB:            2048,
			DisableDefaultStealthArgs: true,
		},
	}

	args := BuildBrowserArgs(cfg, 9222)
	for _, want := range []string{
		"--fingerprint=42069",
		"--fingerprint-platform=windows",
		"--fingerprint-locale=en-GB",
		"--fingerprint-timezone=Europe/London",
		"--fingerprint-webrtc-ip=auto",
		"--fingerprint-fonts-dir=/opt/fonts",
		"--fingerprint-storage-quota=2048",
	} {
		if !stealth.HasLaunchArg(args, want) {
			t.Fatalf("BuildBrowserArgs() missing %q in %v", want, args)
		}
	}
	for _, blocked := range []string{
		"--disable-automation",
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
	} {
		if stealth.HasLaunchArg(args, blocked) {
			t.Fatalf("BuildBrowserArgs() included PinchTab stealth arg %q in native Cloak mode: %v", blocked, args)
		}
	}
	if stealth.HasLaunchArgPrefix(args, "--user-agent=") {
		t.Fatalf("BuildBrowserArgs() included PinchTab user-agent override in native Cloak mode: %v", args)
	}
}

func TestBuildBrowserArgs_DefaultChromeProviderKeepsChromeLaunchContract(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		BrowserVersion: "144.0.0.0",
		ExtensionPaths: []string{},
		// Headless is PinchTab's default deployment mode and is also the mode in
		// which the launch contract pins --user-agent: under headless Chrome the
		// native UA leaks "HeadlessChrome/" and its UA-CH is already degraded, so
		// pinning is a net win. In headed mode the contract intentionally leaves
		// --user-agent off to keep Chrome's native, self-consistent UA + UA-CH.
		Headless: true,
	}

	args := BuildBrowserArgs(cfg, 9222)
	for _, want := range []string{
		"--remote-debugging-port=9222",
		"--disable-background-networking",
		"--disable-automation",
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
		"--lang=en-US",
		"--disable-extensions",
	} {
		if !stealth.HasLaunchArg(args, want) {
			t.Fatalf("BuildBrowserArgs() missing Chrome provider arg %q in %v", want, args)
		}
	}
	if !stealth.HasLaunchArgPrefix(args, "--user-agent=Mozilla/5.0") {
		t.Fatalf("BuildBrowserArgs() missing Chrome provider user-agent in %v", args)
	}
	for _, blockedPrefix := range []string{
		"--fingerprint=",
		"--fingerprint-platform=",
		"--fingerprint-locale=",
		"--fingerprint-timezone=",
		"--fingerprint-webrtc-ip=",
		"--fingerprint-fonts-dir=",
		"--fingerprint-storage-quota=",
	} {
		if stealth.HasLaunchArgPrefix(args, blockedPrefix) {
			t.Fatalf("BuildBrowserArgs() included Cloak flag prefix %q in Chrome provider mode: %v", blockedPrefix, args)
		}
	}
}

func TestCloakBrowserFlagArgsMatchesNativeBuildLaunchArgs(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: "cloak",
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed: "parity-seed-42",
			Platform:        "linux",
			Locale:          "en-US",
			Timezone:        "America/New_York",
			WebRTCIP:        "10.0.0.1",
			FontsDir:        "/usr/share/fonts",
			StorageQuotaMB:  256,
		},
	}
	wrapperArgs := CloakBrowserFlagArgs(cfg)

	nativeArgs, _, _ := browsers.MustGet("cloak").BuildLaunchArgs(browsers.LaunchConfig{
		Cloak: browsers.CloakFingerprint{
			FingerprintSeed: cfg.Cloak.FingerprintSeed,
			Platform:        cfg.Cloak.Platform,
			Locale:          cfg.Cloak.Locale,
			Timezone:        cfg.Cloak.Timezone,
			WebRTCIP:        cfg.Cloak.WebRTCIP,
			FontsDir:        cfg.Cloak.FontsDir,
			StorageQuotaMB:  cfg.Cloak.StorageQuotaMB,
		},
	})

	// Extract only fingerprint flags from native output
	var nativeFP []string
	for _, a := range nativeArgs {
		if strings.HasPrefix(a, "--fingerprint") {
			nativeFP = append(nativeFP, a)
		}
	}

	if len(wrapperArgs) != len(nativeFP) {
		t.Fatalf("wrapper produced %d args, native produced %d fingerprint args\nwrapper: %v\nnative:  %v", len(wrapperArgs), len(nativeFP), wrapperArgs, nativeFP)
	}
	for i := range wrapperArgs {
		if wrapperArgs[i] != nativeFP[i] {
			t.Errorf("arg[%d] mismatch: wrapper=%q native=%q", i, wrapperArgs[i], nativeFP[i])
		}
	}
}

func TestCloakBrowserFlagArgsNilForNonCloak(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: "chrome"}
	if got := CloakBrowserFlagArgs(cfg); len(got) != 0 {
		t.Errorf("expected nil/empty for chrome provider; got %v", got)
	}
	if got := CloakBrowserFlagArgs(nil); got != nil {
		t.Errorf("expected nil for nil config; got %v", got)
	}
}

// Stub browser that does NOT support remote CDP (for guard tests)

type noCDPBrowser struct{}

func (noCDPBrowser) ID() string                                                  { return "nocdpstub" }
func (noCDPBrowser) DisplayName() string                                         { return "NoCDPStub" }
func (noCDPBrowser) Capabilities() browsers.CapabilitySet                        { return browsers.CapabilitySet{} }
func (noCDPBrowser) DiscoverBinary() browsers.BinaryDiscovery                    { return browsers.BinaryDiscovery{} }
func (noCDPBrowser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck { return nil }
func (noCDPBrowser) BuildLaunchArgs(_ browsers.LaunchConfig) ([]string, []string, error) {
	return nil, nil, nil
}
func (noCDPBrowser) SupportsRemoteCDP() bool { return false }
func (noCDPBrowser) GeoAlignment(_ browsers.GeoConfig) browsers.GeoStrategy {
	return browsers.GeoStrategy{}
}
func (noCDPBrowser) ValidateTarget(_ browsers.TargetConfig) error { return nil }
func (noCDPBrowser) ClassifyLaunchError(_ browsers.LaunchFailure) browsers.LaunchErrorKind {
	return browsers.LaunchErrorUnknown
}
func (noCDPBrowser) CanHandle(_ browsers.RequestIntent) browsers.HandleDecision {
	return browsers.HandleDecision{Decision: browsers.DecisionHandle}
}
func (noCDPBrowser) NewRuntimeInstance(_ context.Context, _ bool) browsers.RuntimeInstance {
	return nil
}

var registerNoCDPOnce sync.Once

func TestResolveProviderLaunchPlan_UsesProviderDiscovery(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserCloak}
	want := browsers.MustGet(config.BrowserCloak).DiscoverBinary().Found

	plan, err := resolveProviderLaunchPlan(cfg, providerLaunchConfig(cfg, "", 0))
	if err != nil {
		t.Fatalf("resolveProviderLaunchPlan() error = %v", err)
	}
	if plan.binary != want {
		t.Fatalf("binary = %q, want provider discovery path %q", plan.binary, want)
	}
}

func TestResolveProviderLaunchPlan_PropagatesProviderBuildError(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserCloak}
	_, err := resolveProviderLaunchPlan(cfg, browsers.LaunchConfig{
		Mode: browsers.LaunchModeLite,
	})
	if err == nil {
		t.Fatal("expected provider launch error")
	}
	if !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("error = %v, want provider launch error context", err)
	}
}

func TestInitRemoteCDP_RejectsUnsupportedProvider(t *testing.T) {
	registerNoCDPOnce.Do(func() {
		browsers.Register(&noCDPBrowser{})
	})

	cfg := &config.RuntimeConfig{
		DefaultBrowser: "nocdpstub",
		CDPAttachURL:   "ws://127.0.0.1:9222",
	}
	_, _, _, _, _, err := InitBrowser(cfg, nil, Hooks{})
	if err == nil {
		t.Fatal("expected error for unsupported CDP provider")
	}
	if !strings.Contains(err.Error(), "does not support remote CDP") {
		t.Fatalf("expected 'does not support remote CDP' in error; got: %v", err)
	}
}

// Crash-recovery kill switch (H8 regression): a user-supplied
// --in-process-gpu must be stripped from the direct-launch args when
// DisableInProcessGPU is set, and must appear exactly once when it is not —
// the provider plan must not inject a second, unstrippable copy.
func TestBuildBrowserArgs_DisableInProcessGPUKillSwitch(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser:    config.BrowserChrome,
		BrowserExtraFlags: "--in-process-gpu",
		Headless:          true,
	}

	count := func(args []string) int {
		n := 0
		for _, a := range args {
			if a == "--in-process-gpu" {
				n++
			}
		}
		return n
	}

	if got := count(BuildBrowserArgs(cfg, 9222)); got != 1 {
		t.Fatalf("expected user --in-process-gpu exactly once without kill switch, got %d", got)
	}

	cfg.DisableInProcessGPU = true
	if got := count(BuildBrowserArgs(cfg, 9222)); got != 0 {
		t.Fatalf("kill switch active but --in-process-gpu still present (%d occurrences)", got)
	}
}

// Headless launch args must not contain --disable-gpu on any chrome-family
// path: under --headless=new it removes the compositor's GPU backend and
// Page.captureScreenshot/printToPDF hang.
func TestBuildBrowserArgs_HeadlessOmitsDisableGPU(t *testing.T) {
	for _, provider := range []string{config.BrowserChrome, config.BrowserGhostChrome} {
		cfg := &config.RuntimeConfig{DefaultBrowser: provider, Headless: true}
		for _, a := range BuildBrowserArgs(cfg, 9222) {
			if a == "--disable-gpu" {
				t.Fatalf("provider %s: headless args contain --disable-gpu", provider)
			}
		}
	}
}

// M2 regression: a malformed proxy server must fail the launch-args build —
// launching without the configured proxy would egress from the real IP.
func TestBuildBrowserArgs_MalformedProxyFailsClosed(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		Proxy:          config.BrowserProxyConfig{Server: "not a proxy url"},
	}
	if _, _, err := buildBrowserArgsWithBundle(cfg, nil, 9222, launchGeoAlignment{}); err == nil {
		t.Fatal("expected launch-args build to fail for malformed proxy server")
	}

	cfg.Proxy.Server = "http://proxy.example.com:8080"
	args, _, err := buildBrowserArgsWithBundle(cfg, nil, 9222, launchGeoAlignment{})
	if err != nil {
		t.Fatalf("valid proxy should build: %v", err)
	}
	count := 0
	for _, a := range args {
		if a == "--proxy-server=http://proxy.example.com:8080" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one --proxy-server flag, got %d in %v", count, args)
	}
}
