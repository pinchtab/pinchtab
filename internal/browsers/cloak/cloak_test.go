package cloak_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/cloak"
)

func TestCloakRegistered(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("browsers.Get(\"cloak\") returned false; want registered")
	}
	if b == nil {
		t.Fatal("browsers.Get(\"cloak\") returned nil browser")
	}
}

func TestID(t *testing.T) {
	b, _ := browsers.Get("cloak")
	if got := b.ID(); got != "cloak" {
		t.Errorf("ID() = %q; want %q", got, "cloak")
	}
}

func TestDisplayName(t *testing.T) {
	b, _ := browsers.Get("cloak")
	if got := b.DisplayName(); got != "CloakBrowser" {
		t.Errorf("DisplayName() = %q; want %q", got, "CloakBrowser")
	}
}

func TestCapabilities(t *testing.T) {
	b, _ := browsers.Get("cloak")
	caps := b.Capabilities()

	want := []browsers.BrowserCapability{
		browsers.CapCDP,
		browsers.CapHeadless,
		browsers.CapPDF,
		browsers.CapExtensions,
		browsers.CapDownloads,
		browsers.CapNetworkInterception,
		browsers.CapNativeStealth,
	}

	if got := caps.Len(); got != len(want) {
		t.Fatalf("Capabilities().Len() = %d; want %d", got, len(want))
	}

	for _, c := range want {
		if !caps.Has(c) {
			t.Errorf("Capabilities().Has(%q) = false; want true", c)
		}
	}
}

func TestSupportsRemoteCDP(t *testing.T) {
	b, _ := browsers.Get("cloak")
	if got := b.SupportsRemoteCDP(); !got {
		t.Error("SupportsRemoteCDP() = false; want true (inherited from Chrome)")
	}
}

func TestGeoAlignment(t *testing.T) {
	b, _ := browsers.Get("cloak")
	gs := b.GeoAlignment(browsers.GeoConfig{})

	if len(gs.Flags) != 0 {
		t.Errorf("GeoAlignment().Flags = %v; want empty", gs.Flags)
	}
	if gs.Env != nil {
		t.Errorf("GeoAlignment().Env = %v; want nil", gs.Env)
	}
	if !gs.OperatorWins {
		t.Error("GeoAlignment().OperatorWins = false; want true")
	}
}

func TestBothBrowsersRegistered(t *testing.T) {
	ids := browsers.IDs()

	has := func(target string) bool {
		for _, id := range ids {
			if id == target {
				return true
			}
		}
		return false
	}

	if !has("chrome") {
		t.Errorf("browsers.IDs() missing \"chrome\"; got %v", ids)
	}
	if !has("cloak") {
		t.Errorf("browsers.IDs() missing \"cloak\"; got %v", ids)
	}
}

func TestBinaryNamesReturnsExpectedNames(t *testing.T) {
	names := cloak.BinaryNames()
	if len(names) == 0 {
		t.Fatal("BinaryNames() returned empty slice")
	}
	found := false
	for _, n := range names {
		if n == "cloakbrowser" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("BinaryNames() missing 'cloakbrowser'; got %v", names)
	}

	names[0] = "MUTATED"
	fresh := cloak.BinaryNames()
	if fresh[0] == "MUTATED" {
		t.Fatal("BinaryNames() did not return a defensive copy")
	}
}

func TestCommonPathsPerOS(t *testing.T) {
	linuxPaths := cloak.CommonPaths("linux")
	if len(linuxPaths) == 0 {
		t.Fatal("CommonPaths('linux') returned empty")
	}
	foundCloak := false
	for _, p := range linuxPaths {
		if strings.Contains(p, "cloakbrowser") {
			foundCloak = true
			break
		}
	}
	if !foundCloak {
		t.Errorf("CommonPaths('linux') missing cloakbrowser path; got %v", linuxPaths)
	}

	darwinPaths := cloak.CommonPaths("darwin")
	if len(darwinPaths) == 0 {
		t.Fatal("CommonPaths('darwin') returned empty")
	}

	if cloak.CommonPaths("windows") != nil {
		t.Error("CommonPaths('windows') should return nil")
	}
	if cloak.CommonPaths("freebsd") != nil {
		t.Error("CommonPaths('freebsd') should return nil")
	}
}

func TestBuildLaunchArgsIncludesFingerprintFlags(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	args, _, err := b.BuildLaunchArgs(browsers.LaunchConfig{
		Cloak: browsers.CloakFingerprint{
			FingerprintSeed: "42069",
			Platform:        "linux",
			Locale:          "en-GB",
			Timezone:        "Europe/London",
			WebRTCIP:        "1.2.3.4",
			FontsDir:        "/usr/share/fonts",
			StorageQuotaMB:  512,
		},
	})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	for _, want := range []string{
		"--fingerprint=42069",
		"--fingerprint-platform=linux",
		"--fingerprint-locale=en-GB",
		"--fingerprint-timezone=Europe/London",
		"--fingerprint-webrtc-ip=1.2.3.4",
		"--fingerprint-fonts-dir=/usr/share/fonts",
		"--fingerprint-storage-quota=512",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in args", want)
		}
	}
}

func TestBuildLaunchArgsInheritsChromeBaseFlags(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	args, _, _ := b.BuildLaunchArgs(browsers.LaunchConfig{})
	for _, want := range []string{
		"--disable-background-networking",
		"--disable-metrics-reporting",
		"--password-store=basic",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("cloak BuildLaunchArgs missing Chrome base flag %q", want)
		}
	}
}

func TestBuildLaunchArgsOmitsEmptyFingerprintFields(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	args, _, _ := b.BuildLaunchArgs(browsers.LaunchConfig{
		Cloak: browsers.CloakFingerprint{
			FingerprintSeed: "123",
		},
	})
	if !slices.Contains(args, "--fingerprint=123") {
		t.Fatal("expected --fingerprint=123")
	}
	for _, forbidden := range []string{
		"--fingerprint-platform=",
		"--fingerprint-locale=",
		"--fingerprint-timezone=",
		"--fingerprint-webrtc-ip=",
		"--fingerprint-fonts-dir=",
	} {
		for _, a := range args {
			if strings.HasPrefix(a, forbidden) {
				t.Errorf("empty field should not produce flag %q", a)
			}
		}
	}
}

func TestBuildLaunchArgsStorageQuotaOnlyWhenPositive(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	args, _, _ := b.BuildLaunchArgs(browsers.LaunchConfig{
		Cloak: browsers.CloakFingerprint{StorageQuotaMB: 0},
	})
	for _, a := range args {
		if strings.HasPrefix(a, "--fingerprint-storage-quota=") {
			t.Errorf("StorageQuotaMB=0 should not produce flag, got %q", a)
		}
	}
}

func TestDiscoverBinaryOverridesChrome(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	d := b.DiscoverBinary()

	foundCloakProbe := false
	for _, p := range d.Probed {
		if strings.Contains(p, "cloakbrowser") {
			foundCloakProbe = true
		}
		if strings.Contains(p, "google-chrome") {
			t.Fatal("cloak DiscoverBinary() should not probe for google-chrome")
		}
	}
	if !foundCloakProbe {
		t.Fatal("cloak DiscoverBinary() should probe for cloakbrowser")
	}
}

func TestValidateTargetRequiresBinary(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	err := b.ValidateTarget(browsers.TargetConfig{Binary: ""})
	if err == nil {
		t.Fatal("expected error for empty binary")
	}
}

func TestValidateTargetAcceptsBinary(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	if err := b.ValidateTarget(browsers.TargetConfig{Binary: "/usr/bin/cloakbrowser"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTargetWhitespaceOnlyBinary(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	err := b.ValidateTarget(browsers.TargetConfig{Binary: "   "})
	if err == nil {
		t.Fatal("expected error for whitespace-only binary")
	}
}

func TestClassifyLaunchErrorSilentDrop(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	kind := b.ClassifyLaunchError(browsers.LaunchFailure{
		Err:             context.Canceled,
		Elapsed:         800 * time.Millisecond,
		ParentCanceled:  false,
		BrowserCanceled: true,
	})
	if kind != browsers.LaunchErrorSilentCDPDrop {
		t.Errorf("got %v; want LaunchErrorSilentCDPDrop", kind)
	}
}

func TestClassifyLaunchErrorNilErr(t *testing.T) {
	b, _ := browsers.Get("cloak")
	kind := b.ClassifyLaunchError(browsers.LaunchFailure{})
	if kind != browsers.LaunchErrorUnknown {
		t.Errorf("nil error should return LaunchErrorUnknown; got %v", kind)
	}
}

func TestClassifyLaunchErrorParentCanceled(t *testing.T) {
	b, _ := browsers.Get("cloak")
	kind := b.ClassifyLaunchError(browsers.LaunchFailure{
		Err:             context.Canceled,
		Elapsed:         500 * time.Millisecond,
		ParentCanceled:  true,
		BrowserCanceled: true,
	})
	if kind != browsers.LaunchErrorUnknown {
		t.Errorf("parent canceled should return LaunchErrorUnknown; got %v", kind)
	}
}

func TestClassifyLaunchErrorSlowFailure(t *testing.T) {
	b, _ := browsers.Get("cloak")
	kind := b.ClassifyLaunchError(browsers.LaunchFailure{
		Err:             context.Canceled,
		Elapsed:         6 * time.Second,
		ParentCanceled:  false,
		BrowserCanceled: true,
	})
	if kind != browsers.LaunchErrorUnknown {
		t.Errorf("slow failure should return LaunchErrorUnknown; got %v", kind)
	}
}

func TestClassifyLaunchErrorBrowserNotCanceled(t *testing.T) {
	b, _ := browsers.Get("cloak")
	kind := b.ClassifyLaunchError(browsers.LaunchFailure{
		Err:             context.Canceled,
		Elapsed:         500 * time.Millisecond,
		ParentCanceled:  false,
		BrowserCanceled: false,
	})
	if kind != browsers.LaunchErrorUnknown {
		t.Errorf("browser not canceled should return LaunchErrorUnknown; got %v", kind)
	}
}

func TestBuildLaunchArgsParityWithRepresentativeConfigs(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	chromeBrowser, ok := browsers.Get("chrome")
	if !ok {
		t.Fatal("chrome not registered")
	}

	tests := []struct {
		name   string
		cfg    browsers.LaunchConfig
		checks func(t *testing.T, args []string)
	}{
		{
			name: "full_cloak_config",
			cfg: browsers.LaunchConfig{
				Headless:       true,
				DebugPort:      9222,
				ProfileDir:     "/tmp/cloak-profile",
				Timezone:       "America/New_York",
				NoSandbox:      true,
				ExtensionPaths: []string{"/ext/one"},
				Cloak: browsers.CloakFingerprint{
					FingerprintSeed: "seed-42",
					Platform:        "linux",
					Locale:          "en-US",
					Timezone:        "America/New_York",
					WebRTCIP:        "10.0.0.1",
					FontsDir:        "/usr/share/fonts",
					StorageQuotaMB:  256,
				},
			},
			checks: func(t *testing.T, args []string) {
				t.Helper()
				if !slices.Contains(args, "--disable-background-networking") {
					t.Error("missing Chrome base flag --disable-background-networking")
				}
				if !slices.Contains(args, "--headless=new") {
					t.Error("missing headless flag --headless=new")
				}
				if !slices.Contains(args, "--remote-debugging-port=9222") {
					t.Error("missing --remote-debugging-port=9222")
				}
				if !slices.Contains(args, "--user-data-dir=/tmp/cloak-profile") {
					t.Error("missing --user-data-dir=/tmp/cloak-profile")
				}
				if !slices.Contains(args, "--no-sandbox") {
					t.Error("missing --no-sandbox")
				}
				for _, want := range []string{
					"--fingerprint=seed-42",
					"--fingerprint-platform=linux",
					"--fingerprint-locale=en-US",
					"--fingerprint-timezone=America/New_York",
					"--fingerprint-webrtc-ip=10.0.0.1",
					"--fingerprint-fonts-dir=/usr/share/fonts",
					"--fingerprint-storage-quota=256",
				} {
					if !slices.Contains(args, want) {
						t.Errorf("missing fingerprint flag %q", want)
					}
				}
			},
		},
		{
			name: "minimal_cloak_config",
			cfg: browsers.LaunchConfig{
				Cloak: browsers.CloakFingerprint{
					FingerprintSeed: "minimal-seed",
				},
			},
			checks: func(t *testing.T, args []string) {
				t.Helper()
				if !slices.Contains(args, "--disable-background-networking") {
					t.Error("missing Chrome base flag --disable-background-networking")
				}
				if !slices.Contains(args, "--fingerprint=minimal-seed") {
					t.Error("missing --fingerprint=minimal-seed")
				}
				for _, a := range args {
					if a == "--fingerprint=minimal-seed" {
						continue
					}
					if strings.HasPrefix(a, "--fingerprint") {
						t.Errorf("unexpected fingerprint flag %q in minimal config", a)
					}
				}
				for _, a := range args {
					if strings.HasPrefix(a, "--fingerprint-storage-quota") {
						t.Errorf("unexpected storage quota flag %q in minimal config", a)
					}
				}
			},
		},
		{
			name: "empty_cloak_fingerprint",
			cfg:  browsers.LaunchConfig{},
			checks: func(t *testing.T, args []string) {
				t.Helper()
				if !slices.Contains(args, "--disable-background-networking") {
					t.Error("missing Chrome base flag --disable-background-networking")
				}
				for _, a := range args {
					if strings.HasPrefix(a, "--fingerprint") {
						t.Errorf("unexpected fingerprint flag %q for zero-valued CloakFingerprint", a)
					}
				}
				// Output should be identical to Chrome's BuildLaunchArgs
				// (window-size is randomised per call, so we compare
				// everything except that flag).
				chromeArgs, _, _ := chromeBrowser.BuildLaunchArgs(browsers.LaunchConfig{})
				strip := func(s []string) []string {
					var out []string
					for _, a := range s {
						if !strings.HasPrefix(a, "--window-size=") {
							out = append(out, a)
						}
					}
					return out
				}
				cloakStripped := strip(args)
				chromeStripped := strip(chromeArgs)
				if len(cloakStripped) != len(chromeStripped) {
					t.Fatalf("cloak produced %d args vs chrome %d args for empty config (excluding window-size)\ncloak:  %v\nchrome: %v", len(cloakStripped), len(chromeStripped), cloakStripped, chromeStripped)
				}
				for i := range cloakStripped {
					if cloakStripped[i] != chromeStripped[i] {
						t.Errorf("arg[%d] mismatch: cloak=%q chrome=%q", i, cloakStripped[i], chromeStripped[i])
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _, err := b.BuildLaunchArgs(tt.cfg)
			if err != nil {
				t.Fatalf("BuildLaunchArgs() error = %v", err)
			}
			tt.checks(t, args)
		})
	}
}

func TestGeoAlignmentDerivedFlags(t *testing.T) {
	b, _ := browsers.Get("cloak")
	gs := b.GeoAlignment(browsers.GeoConfig{
		Timezone: "Europe/London",
		Locale:   "en-GB",
		WebRTCIP: "1.2.3.4",
	})
	if len(gs.Flags) != 3 {
		t.Fatalf("expected 3 flags; got %d: %v", len(gs.Flags), gs.Flags)
	}
	want := []string{
		"--fingerprint-timezone=Europe/London",
		"--fingerprint-locale=en-GB",
		"--fingerprint-webrtc-ip=1.2.3.4",
	}
	for i, w := range want {
		if gs.Flags[i] != w {
			t.Errorf("flag[%d] = %q; want %q", i, gs.Flags[i], w)
		}
	}
	if !gs.OperatorWins {
		t.Error("OperatorWins should be true")
	}
}

func TestGeoAlignmentOperatorWins(t *testing.T) {
	b, _ := browsers.Get("cloak")
	gs := b.GeoAlignment(browsers.GeoConfig{
		Timezone:         "Europe/London",
		Locale:           "en-GB",
		WebRTCIP:         "1.2.3.4",
		OperatorTimezone: "America/New_York",
	})
	if len(gs.Flags) != 2 {
		t.Fatalf("expected 2 flags (timezone skipped); got %d: %v", len(gs.Flags), gs.Flags)
	}
	for _, f := range gs.Flags {
		if strings.Contains(f, "timezone") {
			t.Errorf("timezone flag should be skipped when operator set; got %q", f)
		}
	}
}

func TestGeoAlignmentAllOperatorSet(t *testing.T) {
	b, _ := browsers.Get("cloak")
	gs := b.GeoAlignment(browsers.GeoConfig{
		Timezone:         "Europe/London",
		Locale:           "en-GB",
		WebRTCIP:         "1.2.3.4",
		OperatorTimezone: "America/New_York",
		OperatorLocale:   "en-US",
		OperatorWebRTCIP: "10.0.0.1",
	})
	if len(gs.Flags) != 0 {
		t.Errorf("expected no flags when all operator values set; got %v", gs.Flags)
	}
}

func TestGeoAlignmentEmptyGeoConfig(t *testing.T) {
	b, _ := browsers.Get("cloak")
	gs := b.GeoAlignment(browsers.GeoConfig{})
	if len(gs.Flags) != 0 {
		t.Errorf("expected no flags for empty GeoConfig; got %v", gs.Flags)
	}
}

func TestBuildLaunchArgsFingerprintFlagOrder(t *testing.T) {
	b, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	args, _, err := b.BuildLaunchArgs(browsers.LaunchConfig{
		Cloak: browsers.CloakFingerprint{FingerprintSeed: "order-test"},
	})
	if err != nil {
		t.Fatalf("BuildLaunchArgs() error = %v", err)
	}

	baseFlagIdx := -1
	fpFlagIdx := -1
	for i, a := range args {
		if a == "--disable-background-networking" {
			baseFlagIdx = i
		}
		if a == "--fingerprint=order-test" {
			fpFlagIdx = i
		}
	}
	if baseFlagIdx < 0 {
		t.Fatal("Chrome base flag --disable-background-networking not found")
	}
	if fpFlagIdx < 0 {
		t.Fatal("fingerprint flag --fingerprint=order-test not found")
	}
	if fpFlagIdx <= baseFlagIdx {
		t.Errorf("fingerprint flag (index %d) should come after Chrome base flag (index %d)", fpFlagIdx, baseFlagIdx)
	}
}

func TestValidateTargetChromeVsCloakParity(t *testing.T) {
	chromeBrowser, ok := browsers.Get("chrome")
	if !ok {
		t.Fatal("chrome not registered")
	}
	cloakBrowser, ok := browsers.Get("cloak")
	if !ok {
		t.Fatal("cloak not registered")
	}
	cfg := browsers.TargetConfig{Binary: ""}

	if err := chromeBrowser.ValidateTarget(cfg); err != nil {
		t.Fatalf("Chrome should accept empty binary; got %v", err)
	}
	if err := cloakBrowser.ValidateTarget(cfg); err == nil {
		t.Fatal("Cloak should reject empty binary")
	}
}

func TestGeoAlignmentPrecedenceMatrix(t *testing.T) {
	b, _ := browsers.Get("cloak")
	derived := browsers.GeoConfig{
		Timezone: "Europe/London",
		Locale:   "en-GB",
		WebRTCIP: "1.2.3.4",
	}

	tests := []struct {
		name               string
		opTZ, opLoc, opRTC string
		wantFlags          []string
		wantAbsent         []string
	}{
		{
			name: "all_derived",
			wantFlags: []string{
				"--fingerprint-timezone=Europe/London",
				"--fingerprint-locale=en-GB",
				"--fingerprint-webrtc-ip=1.2.3.4",
			},
		},
		{
			name:       "operator_timezone_only",
			opTZ:       "America/New_York",
			wantFlags:  []string{"--fingerprint-locale=en-GB", "--fingerprint-webrtc-ip=1.2.3.4"},
			wantAbsent: []string{"--fingerprint-timezone"},
		},
		{
			name:       "operator_locale_only",
			opLoc:      "en-US",
			wantFlags:  []string{"--fingerprint-timezone=Europe/London", "--fingerprint-webrtc-ip=1.2.3.4"},
			wantAbsent: []string{"--fingerprint-locale"},
		},
		{
			name:       "operator_webrtc_only",
			opRTC:      "10.0.0.1",
			wantFlags:  []string{"--fingerprint-timezone=Europe/London", "--fingerprint-locale=en-GB"},
			wantAbsent: []string{"--fingerprint-webrtc-ip"},
		},
		{
			name: "operator_tz_and_locale",
			opTZ: "x", opLoc: "x",
			wantFlags:  []string{"--fingerprint-webrtc-ip=1.2.3.4"},
			wantAbsent: []string{"--fingerprint-timezone", "--fingerprint-locale"},
		},
		{
			name: "operator_tz_and_webrtc",
			opTZ: "x", opRTC: "x",
			wantFlags:  []string{"--fingerprint-locale=en-GB"},
			wantAbsent: []string{"--fingerprint-timezone", "--fingerprint-webrtc-ip"},
		},
		{
			name:  "operator_locale_and_webrtc",
			opLoc: "x", opRTC: "x",
			wantFlags:  []string{"--fingerprint-timezone=Europe/London"},
			wantAbsent: []string{"--fingerprint-locale", "--fingerprint-webrtc-ip"},
		},
		{
			name: "all_operator",
			opTZ: "x", opLoc: "x", opRTC: "x",
			wantFlags: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gc := derived
			gc.OperatorTimezone = tt.opTZ
			gc.OperatorLocale = tt.opLoc
			gc.OperatorWebRTCIP = tt.opRTC
			gs := b.GeoAlignment(gc)
			if len(gs.Flags) != len(tt.wantFlags) {
				t.Fatalf("got %d flags %v; want %d %v", len(gs.Flags), gs.Flags, len(tt.wantFlags), tt.wantFlags)
			}
			for _, w := range tt.wantFlags {
				found := false
				for _, f := range gs.Flags {
					if f == w {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing expected flag %q in %v", w, gs.Flags)
				}
			}
			for _, a := range tt.wantAbsent {
				for _, f := range gs.Flags {
					if strings.HasPrefix(f, a) {
						t.Errorf("flag %q should be absent; got %q", a, f)
					}
				}
			}
		})
	}
}

func TestCloak_CanHandle(t *testing.T) {
	b := &cloak.Browser{}
	result := b.CanHandle(browsers.RequestIntent{Shape: "visual"})
	if result.Decision != browsers.DecisionHandle {
		t.Errorf("Cloak CanHandle = %q, want %q", result.Decision, browsers.DecisionHandle)
	}
}

func TestCloak_CanHandle_AllShapes(t *testing.T) {
	b := &cloak.Browser{}
	shapes := []string{
		browsers.ShapeStaticRead, browsers.ShapeStaticSnapshot,
		browsers.ShapeRenderedRead, browsers.ShapeVisual,
		browsers.ShapeInteraction, browsers.ShapeSessionState,
		browsers.ShapeNetworkControl, browsers.ShapeDownloadUpload,
		"unknown", "",
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			got := b.CanHandle(browsers.RequestIntent{Shape: shape})
			if got.Decision != browsers.DecisionHandle {
				t.Errorf("CanHandle(%q).Decision = %q, want %q", shape, got.Decision, browsers.DecisionHandle)
			}
		})
	}
}

func TestCloak_CanHandle_StateChanging(t *testing.T) {
	b := &cloak.Browser{}
	got := b.CanHandle(browsers.RequestIntent{Shape: browsers.ShapeStaticRead, StateChanging: true})
	if got.Decision != browsers.DecisionHandle {
		t.Errorf("CanHandle(state-changing).Decision = %q, want %q", got.Decision, browsers.DecisionHandle)
	}
}

// TestCloakPresentHonorsBrowserBinaryOverride is the original report: the
// CloakBrowser installer drops its Chromium in a versioned dir that isn't on the
// static probe list, so a configured browser.binary must PASS cloakbrowser_present
// rather than report a false "cloakbrowser not found" FAIL.
func TestCloakPresentHonorsBrowserBinaryOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cloakbrowser_present is skipped on windows")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "chromium-145.0.0.0", "chrome")
	if err := os.MkdirAll(filepath.Dir(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'Chromium 145.0.0.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	checks := (&cloak.Browser{}).DoctorChecks(browsers.TargetConfig{})
	var present *browsers.DoctorCheck
	for i := range checks {
		if checks[i].ID == "cloakbrowser_present" {
			present = &checks[i]
			break
		}
	}
	if present == nil {
		t.Fatal("cloakbrowser_present check not found")
	}
	res := present.Fn(context.Background(), &browsers.DoctorEnv{Binary: fake})
	if res.Status != browsers.DoctorPass {
		t.Fatalf("cloakbrowser_present with override status = %v, want DoctorPass; detail: %s", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, fake) {
		t.Errorf("expected detail to reference the override path %q; got %q", fake, res.Detail)
	}
}

func TestCloakHandleDecisionsCheck(t *testing.T) {
	b := &cloak.Browser{}
	checks := b.DoctorChecks(browsers.TargetConfig{})
	var found *browsers.DoctorCheck
	for i := range checks {
		if checks[i].ID == "handle_decisions" {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("handle_decisions check not found in Cloak DoctorChecks")
	}
	if found.Description == "" {
		t.Error("handle_decisions check has empty description")
	}
	result := found.Fn(context.Background(), nil)
	if result.Status != browsers.DoctorPass {
		t.Errorf("handle_decisions status = %v, want DoctorPass; detail: %s", result.Status, result.Detail)
	}
	if result.Detail != "all 8 request shapes handled" {
		t.Errorf("handle_decisions detail = %q, want %q", result.Detail, "all 8 request shapes handled")
	}
}

func TestBuildLaunchArgs_RejectsLiteMode(t *testing.T) {
	b, _ := browsers.Get("cloak")
	_, _, err := b.BuildLaunchArgs(browsers.LaunchConfig{Mode: browsers.LaunchModeLite})
	if err == nil {
		t.Fatal("expected error for lite mode")
	}
	if !strings.Contains(err.Error(), "cloak") || !strings.Contains(err.Error(), "lite") {
		t.Errorf("error should mention provider and mode; got: %v", err)
	}
}
