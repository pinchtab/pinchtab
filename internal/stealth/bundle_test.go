package stealth

import (
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestNewBundleIncludesSeedLevelAndPopupGuard(t *testing.T) {
	bundle := NewBundle(&config.RuntimeConfig{StealthLevel: "medium"}, 1234)
	if bundle == nil {
		t.Fatal("expected non-nil bundle")
		return
	}
	if bundle.Level != LevelMedium {
		t.Fatalf("expected level medium, got %s", bundle.Level)
	}
	for _, want := range []string{
		"var __pinchtab_seed = 1234;",
		`var __pinchtab_stealth_level = "medium";`,
		"var __pinchtab_headless = false;",
		"var __pinchtab_profile = ",
		"window.open",
		"window.opener",
	} {
		if !strings.Contains(bundle.Script, want) {
			t.Fatalf("expected bundle script to contain %q", want)
		}
	}
	if !strings.HasPrefix(bundle.ScriptHash, "sha256:") {
		t.Fatalf("expected script hash prefix, got %q", bundle.ScriptHash)
	}
}

func TestScriptHashStableAcrossSeeds(t *testing.T) {
	cfg := &config.RuntimeConfig{StealthLevel: "full", ChromeVersion: "144.0.7559.133"}
	first := NewBundle(cfg, 111)
	second := NewBundle(cfg, 222)
	if first.ScriptHash != second.ScriptHash {
		t.Fatalf("expected script hash to stay stable across seeds, got %q vs %q", first.ScriptHash, second.ScriptHash)
	}
	if first.Script == second.Script {
		t.Fatalf("expected runtime script to still vary with seed")
	}
}

func TestStatusFromBundleReflectsCurrentCapabilityShape(t *testing.T) {
	cfg := &config.RuntimeConfig{StealthLevel: "full", Headless: true}
	bundle := NewBundle(cfg, 7)
	status := StatusFromBundle(bundle, cfg, LaunchModeAllocator)
	if status == nil {
		t.Fatal("expected non-nil status")
		return
	}
	if !status.Capabilities["webglSpoofing"] {
		t.Fatal("expected full mode to report webgl spoofing")
	}
	if !status.Capabilities["webdriverNativeStrategy"] {
		t.Fatal("expected current status to report native webdriver strategy")
	}
	if !status.Capabilities["downlinkMax"] {
		t.Fatal("expected light/full baseline to report downlinkMax capability")
	}
	if status.Capabilities["iframeIsolation"] {
		t.Fatal("expected current full mode to keep iframe isolation capability disabled")
	}
	if !status.Capabilities["errorStackSanitized"] {
		t.Fatal("expected full mode to report stack sanitization")
	}
	if !status.Capabilities["functionToStringMasked"] {
		t.Fatal("expected full mode to report function-toString masking")
	}
	if !status.Capabilities["functionToStringNative"] {
		t.Fatal("expected full mode to report native Function.prototype.toString semantics")
	}
	if !status.Capabilities["intlLocaleCoherent"] {
		t.Fatal("expected full mode to report locale coherence capability")
	}
	if !status.Capabilities["errorPrepareStackTraceNative"] {
		t.Fatal("expected full mode to report native Error.prepareStackTrace semantics")
	}
	if status.Capabilities["systemColorFix"] {
		t.Fatal("expected current full mode to keep system color wrappers disabled")
	}
	if status.Capabilities["videoCodecs"] {
		t.Fatal("expected current full mode to keep codec spoofing disabled")
	}
	if status.Capabilities["canvasNoise"] {
		t.Fatal("expected full mode to keep canvas noise disabled in the current public-site profile")
	}
	if status.Capabilities["transparentPixelCanvasNoise"] {
		t.Fatal("expected full mode to keep transparent pixel canvas noise disabled in the current public-site profile")
	}
	if status.Capabilities["audioNoise"] {
		t.Fatal("expected full mode to keep audio noise disabled in the current public-site profile")
	}
	if status.Capabilities["webrtcMitigation"] {
		t.Fatal("expected full mode to keep JS WebRTC mitigation disabled in the current public-site profile")
	}
	if !status.Flags["headlessNew"] {
		t.Fatal("expected headlessNew flag to be true for headless config")
	}
}

func TestStatusFromBundleDisablesWebGLSpoofingWhenHeaded(t *testing.T) {
	cfg := &config.RuntimeConfig{StealthLevel: "full", Headless: false}
	bundle := NewBundle(cfg, 7)
	status := StatusFromBundle(bundle, cfg, LaunchModeAllocator)
	if status == nil {
		t.Fatal("expected non-nil status")
		return
	}
	if status.Capabilities["webglSpoofing"] {
		t.Fatal("expected headed full mode to avoid WebGL spoofing")
	}
}

func TestResolveUserAgent(t *testing.T) {
	if got := ResolveUserAgent("custom-agent", "144.0.0.0"); got != "custom-agent" {
		t.Fatalf("expected explicit UA to win, got %q", got)
	}
	got := ResolveUserAgent("", "144.0.0.0")
	if !strings.Contains(got, "Chrome/144.0.0.0") {
		t.Fatalf("expected generated UA to include chrome version, got %q", got)
	}
}

func TestBuildLaunchContractOwnsStealthLaunchFlags(t *testing.T) {
	launch := BuildLaunchContract(&config.RuntimeConfig{ChromeVersion: "144.0.0.0"}, LevelLight)
	for _, want := range []string{
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
		"--lang=en-US",
	} {
		if !HasLaunchArg(launch.Args, want) {
			t.Fatalf("expected stealth launch arg %q in %v", want, launch.Args)
		}
	}
	// Without an explicit custom UA, --user-agent must NOT be pinned (pinning it
	// empties Chrome's native high-entropy UA Client Hints).
	if HasLaunchArgPrefix(launch.Args, "--user-agent=") {
		t.Fatalf("did not expect a pinned user-agent without a custom UA, got %v", launch.Args)
	}
	// With an explicit custom UA, the launch contract owns --user-agent.
	custom := BuildLaunchContract(&config.RuntimeConfig{ChromeVersion: "144.0.0.0", UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"}, LevelLight)
	if !HasLaunchArgPrefix(custom.Args, "--user-agent=Mozilla/5.0") {
		t.Fatalf("expected an explicit custom UA to pin --user-agent, got %v", custom.Args)
	}
}

func TestNewBundleNativeCloakDisablesPinchTabStealthOverlays(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserCloak,
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed:           "42069",
			DisableDefaultStealthArgs: true,
		},
		StealthLevel: "full",
		Headless:     true,
	}

	bundle := NewBundle(cfg, 1234)
	if bundle.Provider != config.BrowserCloak {
		t.Fatalf("Provider = %q, want %q", bundle.Provider, config.BrowserCloak)
	}
	if !bundle.Native {
		t.Fatal("expected native Cloak bundle")
	}
	if !bundle.PinchTabOverlaysDisabled {
		t.Fatal("expected PinchTab overlays to be disabled")
	}
	if strings.Contains(bundle.Script, "__pinchtab_stealth_level") {
		t.Fatalf("native Cloak script should not include PinchTab JS stealth overlay")
	}
	if !strings.Contains(bundle.Script, "window.open") {
		t.Fatalf("native Cloak script should retain popup guard")
	}
	if len(bundle.Launch.Args) != 0 {
		t.Fatalf("native Cloak launch args = %v, want none", bundle.Launch.Args)
	}
	if !bundle.Launch.Flags["pinchtabStealthArgsDisabled"] {
		t.Fatalf("native Cloak launch flags = %v, want pinchtabStealthArgsDisabled", bundle.Launch.Flags)
	}

	status := StatusFromBundle(bundle, cfg, LaunchModeAllocator)
	if status.Provider != config.BrowserCloak || !status.Native || !status.PinchTabOverlaysDisabled {
		t.Fatalf("status = %+v, want native cloak provider with overlays disabled", status)
	}
	if status.FingerprintSeed != "42069" {
		t.Fatalf("status FingerprintSeed = %q, want 42069", status.FingerprintSeed)
	}
	if !status.Capabilities["sourceLevelFingerprinting"] {
		t.Fatalf("status capabilities = %v, want sourceLevelFingerprinting", status.Capabilities)
	}
}

func TestNewBundleCloakProviderCanKeepPinchTabStealthOverlays(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserCloak,
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed:           "42069",
			DisableDefaultStealthArgs: false,
		},
		StealthLevel: "full",
		Headless:     true,
	}

	bundle := NewBundle(cfg, 1234)
	if !bundle.Native {
		t.Fatal("expected Cloak provider to report native Cloak mode")
	}
	if bundle.PinchTabOverlaysDisabled {
		t.Fatal("expected PinchTab overlays to remain enabled")
	}
	if !strings.Contains(bundle.Script, "__pinchtab_stealth_level") {
		t.Fatal("expected PinchTab JS stealth overlay to remain in bundle")
	}
	if len(bundle.Launch.Args) == 0 {
		t.Fatalf("expected PinchTab launch args to remain enabled")
	}
	if !bundle.Launch.Flags["nativeCloakBrowser"] {
		t.Fatalf("launch flags = %v, want nativeCloakBrowser", bundle.Launch.Flags)
	}
	if bundle.Launch.Flags["pinchtabStealthArgsDisabled"] {
		t.Fatalf("launch flags = %v, did not expect pinchtabStealthArgsDisabled", bundle.Launch.Flags)
	}

	status := StatusFromBundle(bundle, cfg, LaunchModeAllocator)
	if status.Provider != config.BrowserCloak || !status.Native || status.PinchTabOverlaysDisabled {
		t.Fatalf("status = %+v, want native cloak provider with overlays enabled", status)
	}
}

func TestStatusFromBundleEchoesProviderCapabilities(t *testing.T) {
	t.Run("chrome", func(t *testing.T) {
		cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserChrome}
		bundle := NewBundle(cfg, 1)
		status := StatusFromBundle(bundle, cfg, LaunchModeAllocator)
		if status == nil {
			t.Fatal("expected non-nil status")
			return
		}
		want := []string{"cdp", "downloads", "extensions", "headless", "networkInterception", "pdf"}
		if !equalStringSlices(status.ProviderCapabilities, want) {
			t.Fatalf("chrome ProviderCapabilities = %v, want %v", status.ProviderCapabilities, want)
		}
		for _, c := range want {
			if c == "nativeStealth" {
				t.Fatalf("chrome should not advertise nativeStealth")
			}
		}
	})

	t.Run("cloak", func(t *testing.T) {
		cfg := &config.RuntimeConfig{
			DefaultBrowser: config.BrowserCloak,
			Cloak: config.CloakBrowserRuntimeConfig{
				DisableDefaultStealthArgs: true,
			},
		}
		bundle := NewBundle(cfg, 1)
		status := StatusFromBundle(bundle, cfg, LaunchModeAllocator)
		if status == nil {
			t.Fatal("expected non-nil status")
			return
		}
		want := []string{"cdp", "downloads", "extensions", "headless", "nativeStealth", "networkInterception", "pdf"}
		if !equalStringSlices(status.ProviderCapabilities, want) {
			t.Fatalf("cloak ProviderCapabilities = %v, want %v", status.ProviderCapabilities, want)
		}
	})
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
