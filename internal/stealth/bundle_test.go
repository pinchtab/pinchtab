package stealth

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
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
	cfg := &config.RuntimeConfig{StealthLevel: "full", BrowserVersion: "144.0.7559.133"}
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
	if got := resolveUserAgent("custom-agent", "144.0.0.0"); got != "custom-agent" {
		t.Fatalf("expected explicit UA to win, got %q", got)
	}
	got := resolveUserAgent("", "144.0.0.0")
	if !strings.Contains(got, "Chrome/144.0.0.0") {
		t.Fatalf("expected generated UA to include chrome version, got %q", got)
	}
	if want := ChromeUserAgent(HostPlatform(), "144.0.0.0"); got != want {
		t.Fatalf("generated UA = %q, want the shared template's %q", got, want)
	}
}

// One template, three platforms, and the OS tokens spelled out here so a change to
// either side has to be deliberate. Chrome's UA reduction has moved these frozen
// tokens before; when it moves them again this is the test that says so.
func TestChromeUserAgentIsTheOnlyTemplate(t *testing.T) {
	for _, tc := range []struct{ platform, want string }{
		{PlatformWindows, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"},
		{PlatformMacOS, "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"},
		{PlatformLinux, "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			if got := ChromeUserAgent(tc.platform, "144.0.0.0"); got != tc.want {
				t.Errorf("ChromeUserAgent(%q) =\n %q\nwant\n %q", tc.platform, got, tc.want)
			}
		})
	}

	// An unknown platform falls to Linux rather than returning an empty UA: a
	// persona with no user agent is a worse answer than a plausible one.
	if got, want := ChromeUserAgent("Android", "144.0.0.0"), ChromeUserAgent(PlatformLinux, "144.0.0.0"); got != want {
		t.Errorf("unknown platform = %q, want the Linux template %q", got, want)
	}
}

// Edge is a decoration over the Chrome result, not a fourth template — asserted by
// composition so the OS tokens cannot appear in a second place.
func TestEdgeUserAgentDecoratesTheChromeTemplate(t *testing.T) {
	chrome := ChromeUserAgent(PlatformWindows, "144.0.0.0")

	got := EdgeUserAgent(chrome, "144.0.0.0")

	if want := chrome + " Edg/144.0.0.0"; got != want {
		t.Fatalf("EdgeUserAgent = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, chrome) {
		t.Errorf("Edge UA %q no longer starts with the Chrome UA it decorates", got)
	}
}

// The platform is read back out of the UA string, which looks like a round trip on
// the generated path and is load-bearing on the custom-UA path: nobody selected a
// platform there, so the string is the only thing that knows. A custom Windows UA on
// a mac host must still report Win32 — replacing the sniff with the generated
// platform would report the host's.
func TestCustomUserAgentPlatformComesFromTheStringNotTheHost(t *testing.T) {
	for _, tc := range []struct {
		name              string
		userAgent         string
		navigatorPlatform string
		uaDataPlatform    string
	}{
		{
			name:              "windows UA",
			userAgent:         ChromeUserAgent(PlatformWindows, "144.0.0.0"),
			navigatorPlatform: "Win32",
			uaDataPlatform:    PlatformWindows,
		},
		{
			name:              "macOS UA",
			userAgent:         ChromeUserAgent(PlatformMacOS, "144.0.0.0"),
			navigatorPlatform: "MacIntel",
			uaDataPlatform:    PlatformMacOS,
		},
		{
			name:              "linux UA",
			userAgent:         ChromeUserAgent(PlatformLinux, "144.0.0.0"),
			navigatorPlatform: "Linux x86_64",
			uaDataPlatform:    PlatformLinux,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			persona := BuildPersona(tc.userAgent, "144.0.7559.133")

			if persona.UserAgent != tc.userAgent {
				t.Fatalf("custom UA was rewritten: %q", persona.UserAgent)
			}
			if persona.NavigatorPlatform != tc.navigatorPlatform {
				t.Errorf("navigator.platform = %q, want %q for a UA the caller supplied", persona.NavigatorPlatform, tc.navigatorPlatform)
			}
			if persona.UserAgentData.Platform != tc.uaDataPlatform {
				t.Errorf("userAgentData.platform = %q, want %q", persona.UserAgentData.Platform, tc.uaDataPlatform)
			}
		})
	}
}

func TestBuildLaunchContractOwnsStealthLaunchFlags(t *testing.T) {
	launch := BuildLaunchContract(&config.RuntimeConfig{BrowserVersion: "144.0.0.0"}, LevelLight)
	for _, want := range []string{
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
		"--lang=en-US",
	} {
		if !slices.Contains(launch.Args, want) {
			t.Fatalf("expected stealth launch arg %q in %v", want, launch.Args)
		}
	}
	// Without an explicit custom UA, --user-agent must NOT be pinned (pinning it
	// empties Chrome's native high-entropy UA Client Hints).
	if HasLaunchArgPrefix(launch.Args, "--user-agent=") {
		t.Fatalf("did not expect a pinned user-agent without a custom UA, got %v", launch.Args)
	}
	// With an explicit custom UA, the launch contract owns --user-agent.
	custom := BuildLaunchContract(&config.RuntimeConfig{BrowserVersion: "144.0.0.0", UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"}, LevelLight)
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
	// The PinchTab worker-UA-parity script is disabled for native cloak, so the
	// status must not advertise PinchTab-provided worker UA parity.
	if status.Capabilities["workerUserAgentConsistency"] {
		t.Errorf("native cloak should not advertise workerUserAgentConsistency (PinchTab worker script disabled)")
	}
	if status.Capabilities["serviceWorkerUserAgentConsistency"] {
		t.Errorf("native cloak should not advertise serviceWorkerUserAgentConsistency (PinchTab worker script disabled)")
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
		want := []string{"cdp", "downloads", "eventScreencast", "extensions", "headless", "networkInterception", "pdf", "runtimeConsoleEvents"}
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

// chromeTemplateMarker is the part of the Chrome UA that must exist in exactly one
// place. The parity tests would stay green if a caller kept a byte-identical copy of
// its own — that is how the two copies this consolidation removed passed for as long
// as they agreed — so the census is what makes a second owner impossible rather than
// merely currently-correct.
const chromeTemplateMarker = "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/"

func TestChromeUserAgentTemplateHasOneOwner(t *testing.T) {
	// srccensus.Tree owns the enumeration (with the nested-checkout skip the old
	// walk lacked entirely — it skipped no directory at all). Keys were already
	// module-relative slash paths, so the owner key is unchanged.
	owners := map[string]int{}
	for _, file := range srccensus.Tree(t, filepath.Join("..", ".."), 100) {
		if n := strings.Count(file.Text, chromeTemplateMarker); n > 0 {
			owners[file.Name] = n
		}
	}

	const owner = "internal/stealth/ua.go"
	if len(owners) != 1 || owners[owner] == 0 {
		t.Fatalf("the Chrome UA template is spelled out in %v, want only %s — a second copy drifts on the frozen OS tokens exactly as the fingerprint endpoint's copy drifted on the version", owners, owner)
	}
	// One arm per platform, in one function. A fourth arm means a platform gained a
	// spelling without going through ChromeUserAgent.
	if owners[owner] != 3 {
		t.Errorf("%s spells the template %d times, want 3 (one arm per platform)", owner, owners[owner])
	}
}

// The rotate path now refuses an empty UA at Bridge.SetUserAgentOverride, and that
// guard must not be satisfiable by quietly making the launch override empty too. The
// launch path does not go through it: emulation.go hands persona.UserAgent to
// Emulation directly, which needs no guard for a checked reason rather than an
// assumed one — BuildPersona reduces the version first and ReducedBrowserVersion
// falls back, so the field cannot be empty whatever the configured version is.
func TestTheLaunchOverrideUserAgentIsNeverEmpty(t *testing.T) {
	for _, version := range []string{"", "   ", "144.0.7559.133", "144", "0", "not-a-version"} {
		for _, customUA := range []string{"", "  "} {
			persona := BuildPersona(customUA, version)
			if strings.TrimSpace(persona.UserAgent) == "" {
				t.Errorf("BuildPersona(%q, %q).UserAgent is empty; the launch path would apply it and blank navigator.userAgent", customUA, version)
			}
		}
	}

	// The field asserted above is the one the launch path applies. Without this the
	// test would keep passing after the launch override started reading something else.
	pkg := srccensus.Load(t, ".", 5)
	if !strings.Contains(pkg.Source(pkg.MustFunc(t, "BuildUserAgentOverride")), "emulation.SetUserAgentOverride(persona.UserAgent)") {
		t.Error("BuildUserAgentOverride no longer applies persona.UserAgent, so the non-empty guarantee above is about a field the launch path does not use")
	}
}
