package stealth

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestNewBundleIncludesSeedLevelAndPopupGuard(t *testing.T) {
	bundle := NewBundle(&config.RuntimeConfig{StealthLevel: "medium"}, 1234)
	if bundle == nil {
		t.Fatal("expected non-nil bundle")
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
	if status.Capabilities["errorStackSanitized"] {
		t.Fatal("expected current full mode to keep stack sanitization disabled")
	}
	if status.Capabilities["functionToStringMasked"] {
		t.Fatal("expected current full mode to keep function-toString masking disabled")
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
	if !HasLaunchArgPrefix(launch.Args, "--user-agent=Mozilla/5.0") {
		t.Fatalf("expected stealth launch contract to own user-agent, got %v", launch.Args)
	}
}
