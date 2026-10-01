package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func TestHandleFingerprintRotate_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()

	h.HandleFingerprintRotate(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGenerateFingerprint_Windows(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "120.0.0.0"}}
	fp, err := h.generateFingerprint(fingerprintRequest{OS: "windows"})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	if fp.Platform != "Win32" {
		t.Errorf("expected Win32, got %q", fp.Platform)
	}
	if fp.UserAgent == "" {
		t.Error("expected non-empty user agent")
	}
	if fp.ScreenWidth == 0 || fp.ScreenHeight == 0 {
		t.Error("expected non-zero screen dimensions")
	}
	if fp.Vendor != "Google Inc." {
		t.Errorf("expected Google Inc., got %q", fp.Vendor)
	}
}

func TestGenerateFingerprint_Mac(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "120.0.0.0"}}
	fp, err := h.generateFingerprint(fingerprintRequest{OS: "mac"})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	if fp.Platform != "MacIntel" {
		t.Errorf("expected MacIntel, got %q", fp.Platform)
	}
}

func TestGenerateFingerprint_Random(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "120.0.0.0"}}
	fp, err := h.generateFingerprint(fingerprintRequest{OS: "random"})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	validPlatforms := map[string]bool{"Win32": true, "MacIntel": true}
	if !validPlatforms[fp.Platform] {
		t.Errorf("unexpected platform %q", fp.Platform)
	}
}

func TestGenerateFingerprint_WithBrowser(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "120.0.0.0"}}
	fp, err := h.generateFingerprint(fingerprintRequest{OS: "windows", Browser: "chrome"})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	if fp.UserAgent == "" {
		t.Error("expected non-empty user agent")
	}
}

func TestGenerateFingerprint_Config(t *testing.T) {
	cfg := &config.RuntimeConfig{BrowserVersion: "120.0.0.0"}
	h := Handlers{Config: cfg}

	fp, err := h.generateFingerprint(fingerprintRequest{OS: "windows", Browser: "chrome"})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	if !strings.Contains(fp.UserAgent, "120.0.0.0") {
		t.Errorf("expected User-Agent to contain Chrome version 120.0.0.0, got %q", fp.UserAgent)
	}
}

// The launch path pins navigator.userAgent to Chrome's UA-reduced form
// (Chrome/<major>.0.0.0). /fingerprint/rotate runs against the same browser
// session and MUST emit the same reduced form, otherwise an initial UA of
// Chrome/144.0.0.0 followed by a rotated UA of Chrome/144.0.7559.133 trips
// the "Chrome version preserved" E2E contract (system-basic.sh).
func TestGenerateFingerprint_MatchesLaunchPinnedUAReduction(t *testing.T) {
	cfg := &config.RuntimeConfig{BrowserVersion: "144.0.7559.133", Headless: true}
	bundle := stealth.NewBundle(cfg, 1)
	launchUA := bundle.LaunchUserAgent()
	if launchUA == "" {
		t.Fatalf("precondition: headless launch must pin a UA, got empty")
	}
	if !strings.Contains(launchUA, "Chrome/144.0.0.0") {
		t.Fatalf("precondition: launch UA should be reduced to Chrome/144.0.0.0, got %q", launchUA)
	}

	h := Handlers{Config: cfg}
	for _, tc := range []struct {
		os, browser string
	}{
		{"windows", "chrome"},
		{"mac", "chrome"},
		{"windows", "edge"},
	} {
		fp, err := h.generateFingerprint(fingerprintRequest{OS: tc.os, Browser: tc.browser})
		if err != nil {
			t.Fatalf("generateFingerprint: %v", err)
		}
		if !strings.Contains(fp.UserAgent, "144.0.0.0") {
			t.Errorf("rotate UA for %s/%s should carry reduced Chrome/144.0.0.0, got %q", tc.os, tc.browser, fp.UserAgent)
		}
		if strings.Contains(fp.UserAgent, "144.0.7559.133") {
			t.Errorf("rotate UA for %s/%s leaks full build version (UA reduction violated): %q", tc.os, tc.browser, fp.UserAgent)
		}
	}
}

func TestTimezoneIDFromOffset(t *testing.T) {
	if got := timezoneIDFromOffset(-300); got != "America/New_York" {
		t.Fatalf("timezoneIDFromOffset(-300) = %q, want America/New_York", got)
	}
	if got := timezoneIDFromOffset(999); got != "" {
		t.Fatalf("timezoneIDFromOffset(999) = %q, want empty string", got)
	}
}

func TestFingerprintRotatePlatformOverlayScript(t *testing.T) {
	script := fingerprintRotatePlatformOverlayScript("Win32")
	if !strings.Contains(script, "Object.defineProperty(proto, 'platform'") {
		t.Fatalf("expected platform overlay script to patch navigator platform, got %q", script)
	}
	if !strings.Contains(script, "\"Win32\"") {
		t.Fatalf("expected platform overlay script to embed platform, got %q", script)
	}
}

func TestStealthScript_Content(t *testing.T) {
	if assets.StealthScript == "" {
		t.Fatal("StealthScript is empty")
	}
	if !strings.Contains(assets.StealthScript, "navigator") || !strings.Contains(assets.StealthScript, "webdriver") {
		t.Error("stealth script missing webdriver protection")
	}
	if strings.Contains(assets.StealthScript, "proxyNavigator") || strings.Contains(assets.StealthScript, "Object.defineProperty(window, 'navigator'") {
		t.Error("stealth script should not proxy window.navigator in light mode")
	}
	if !strings.Contains(assets.StealthScript, "downlinkMax") {
		t.Error("stealth script missing downlinkMax coverage")
	}
}

func TestStealthScript_Populated(t *testing.T) {
	b := bridge.New(context.Background(), context.Background(), &config.RuntimeConfig{})

	if b.StealthBundle == nil || b.StealthBundle.Script == "" {
		t.Error("expected stealth bundle script to be populated")
	}
}

func (m *mockBridge) StealthStatus() *stealth.Status {
	return &stealth.Status{
		Level:         stealth.LevelMedium,
		Headless:      true,
		LaunchMode:    stealth.LaunchModeAllocator,
		ScriptHash:    "sha256:test",
		WebdriverMode: stealth.WebdriverModeNativeBaseline,
		Flags: map[string]bool{
			"headlessNew": true,
		},
		Capabilities: map[string]bool{
			"userAgentData":           true,
			"webdriverNativeStrategy": true,
			"downlinkMax":             true,
		},
		TabOverrides: map[string]bool{
			"fingerprintRotateActive": false,
		},
	}
}

func TestHandleStealthStatus(t *testing.T) {
	mb := &mockBridge{fingerprintTabs: map[string]bool{"tab1": true}}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/stealth/status", nil)
	w := httptest.NewRecorder()

	h.HandleStealthStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if got := resp["level"]; got != "medium" {
		t.Fatalf("expected level=medium, got %v", got)
	}
	if got := resp["launchMode"]; got != "allocator" {
		t.Fatalf("expected launchMode=allocator, got %v", got)
	}
}

func TestHandleStealthStatus_WithTabOverride(t *testing.T) {
	mb := &mockBridge{fingerprintTabs: map[string]bool{"tab-special": true}}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/stealth/status?tabId=tab-special", nil)
	w := httptest.NewRecorder()

	h.HandleStealthStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	tabOverrides, ok := resp["tabOverrides"].(map[string]any)
	if !ok {
		t.Fatalf("expected tabOverrides object, got %T", resp["tabOverrides"])
	}
	if got := tabOverrides["fingerprintRotateActive"]; got != true {
		t.Fatalf("expected fingerprintRotateActive=true, got %v", got)
	}
}

// The drift this pins already happened once on the version axis and had to be
// resynced by hand; the OS-token axis was the next one. Comparing against
// stealth.ChromeUserAgent rather than against a literal is what makes it
// impossible rather than merely corrected: there is one template, and if it moves,
// both sides move together or this test fails.
func TestGenerateFingerprintUsesTheSharedChromeTemplate(t *testing.T) {
	const version = "144.0.7559.133"
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: version}}
	reduced := stealth.ReducedBrowserVersion(version)

	for _, tc := range []struct {
		os, browser  string
		wantPlatform string
		wantUA       string
	}{
		{os: "windows", browser: "chrome", wantPlatform: "Win32", wantUA: stealth.ChromeUserAgent(stealth.PlatformWindows, reduced)},
		{os: "mac", browser: "chrome", wantPlatform: "MacIntel", wantUA: stealth.ChromeUserAgent(stealth.PlatformMacOS, reduced)},
		{os: "linux", browser: "chrome", wantPlatform: "Linux x86_64", wantUA: stealth.ChromeUserAgent(stealth.PlatformLinux, reduced)},
		{os: "windows", browser: "edge", wantPlatform: "Win32", wantUA: stealth.EdgeUserAgent(stealth.ChromeUserAgent(stealth.PlatformWindows, reduced), reduced)},
	} {
		t.Run(tc.os+"/"+tc.browser, func(t *testing.T) {
			fp, err := h.generateFingerprint(fingerprintRequest{OS: tc.os, Browser: tc.browser})
			if err != nil {
				t.Fatalf("generateFingerprint: %v", err)
			}

			if fp.UserAgent != tc.wantUA {
				t.Errorf("user agent =\n %q\nwant the shared template's\n %q", fp.UserAgent, tc.wantUA)
			}
			if fp.Platform != tc.wantPlatform {
				t.Errorf("platform = %q, want %q", fp.Platform, tc.wantPlatform)
			}
			// The reduced version is what real Chrome exposes; a full build here is the
			// exact drift the comment above generateFingerprint was written for.
			if strings.Contains(fp.UserAgent, version) {
				t.Errorf("user agent carries the full build %q: %s", version, fp.UserAgent)
			}
		})
	}
}

// The same template serves the launch persona, so the endpoint and the browser
// PinchTab actually launches cannot describe different Chromes on the same host.
func TestGenerateFingerprintAgreesWithTheLaunchPersonaOnThisHost(t *testing.T) {
	const version = "144.0.7559.133"
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: version}}

	fp, err := h.generateFingerprint(fingerprintRequest{OS: stealth.HostFingerprintOS(), Browser: "chrome"})
	if err != nil {
		t.Fatalf("generateFingerprint: %v", err)
	}
	persona := stealth.BuildPersona("", version)

	if fp.UserAgent != persona.UserAgent {
		t.Fatalf("fingerprint endpoint and launch persona disagree on this host:\n endpoint %q\n persona  %q", fp.UserAgent, persona.UserAgent)
	}
	if fp.Platform != persona.NavigatorPlatform {
		t.Errorf("platform = %q, want the persona's %q", fp.Platform, persona.NavigatorPlatform)
	}
}

// os: "random" keeps its windows/mac weighting for every browser a weighted row
// holds — which is every browser the shipped matrix has outside linux/chrome — so
// the default request is untouched. linux is a candidate only when nothing weighted
// holds the requested browser, and chrome is held by both weighted rows, so this
// drives the real matrix and must never see the Linux UA.
func TestGenerateFingerprintRandomStaysWindowsOrMac(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	linuxUA := stealth.ChromeUserAgent(stealth.PlatformLinux, stealth.ReducedBrowserVersion("144.0.7559.133"))

	for range 200 {
		fp, err := h.generateFingerprint(fingerprintRequest{OS: "random"})
		if err != nil {
			t.Fatalf("generateFingerprint: %v", err)
		}
		if fp.UserAgent == linuxUA {
			t.Fatalf("os: random returned the Linux UA; adding linux to the weighted pick changes what a default request returns")
		}
		if fp.Platform != "Win32" && fp.Platform != "MacIntel" {
			t.Fatalf("os: random returned platform %q, want Win32 or MacIntel", fp.Platform)
		}
	}
}

// An unlisted pair used to answer 200 with an empty userAgent. For an endpoint
// whose purpose is to hand back an identity to apply, an empty identity delivered
// as a success is worse than a refusal: the caller cannot tell it received nothing.
// linux+edge and mac+edge are real combinations, which is what makes the silence
// reachable by a reasonable request.
func TestGenerateFingerprintRefusesAnUnlistedPair(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}

	for _, tc := range []struct{ os, browser string }{
		{os: "linux", browser: "edge"},
		{os: "mac", browser: "edge"},
		{os: "windows", browser: "safari"},
		{os: "bogus", browser: "chrome"},
		{os: "linux", browser: "safari"},
	} {
		t.Run(tc.os+"/"+tc.browser, func(t *testing.T) {
			fp, err := h.generateFingerprint(fingerprintRequest{OS: tc.os, Browser: tc.browser})
			if err == nil {
				t.Fatalf("pair was accepted and returned userAgent %q", fp.UserAgent)
			}
			if fp.UserAgent != "" || fp.Platform != "" || fp.Vendor != "" {
				t.Errorf("refused request still carries an identity: %+v", fp)
			}
			for _, want := range []string{tc.os, tc.browser} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name the requested %q", err, want)
				}
			}
			// The pairs that do exist, so the caller can correct itself in one step.
			for _, want := range availableFingerprintPairs(h.fingerprintMatrix()) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q omits the available pair %q", err, want)
				}
			}
		})
	}
}

// A 400 must carry no partial identity: a randomised screen size and core count
// beside a refusal would imply the request was honoured.
func TestARefusedFingerprintPopulatesNoOtherField(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}

	fp, err := h.generateFingerprint(fingerprintRequest{OS: "linux", Browser: "edge", Screen: "random", Language: "fr-FR", Timezone: -60})
	if err == nil {
		t.Fatal("precondition: linux/edge must be refused")
	}
	if fp != (fingerprint{}) {
		t.Errorf("refused request returned a populated fingerprint: %+v", fp)
	}
}

// The list of valid pairs is derived from the matrix, not restated beside it: a pair
// added to the map appears in the message with no second list to edit.
func TestTheRefusalMessageIsDerivedFromTheMatrix(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}

	matrix := h.fingerprintMatrix()
	if got := availableFingerprintPairs(matrix); len(got) == 0 {
		t.Fatal("the matrix is empty — this test would pass vacuously")
	}

	matrix["linux"]["edge"] = fingerprint{UserAgent: "probe", Platform: "Linux x86_64", Vendor: "Google Inc."}
	pairs := availableFingerprintPairs(matrix)

	var found bool
	for _, pair := range pairs {
		if pair == "linux/edge" {
			found = true
		}
	}
	if !found {
		t.Errorf("a pair added to the matrix does not appear in the derived list: %v", pairs)
	}
	if !sort.StringsAreSorted(pairs) {
		t.Errorf("derived pairs are not sorted, so the message varies between runs: %v", pairs)
	}
}

// Every pair that works today must keep working and stay byte-identical, including
// the empty-browser default of chrome.
func TestEveryListedPairStillResolvesByteIdentically(t *testing.T) {
	const version = "144.0.7559.133"
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: version}}
	reduced := stealth.ReducedBrowserVersion(version)
	windowsChrome := stealth.ChromeUserAgent(stealth.PlatformWindows, reduced)

	for _, tc := range []struct {
		name         string
		os, browser  string
		wantUA       string
		wantPlatform string
		wantVendor   string
	}{
		{name: "windows/chrome", os: "windows", browser: "chrome", wantUA: windowsChrome, wantPlatform: "Win32", wantVendor: "Google Inc."},
		{name: "windows/edge", os: "windows", browser: "edge", wantUA: stealth.EdgeUserAgent(windowsChrome, reduced), wantPlatform: "Win32", wantVendor: "Google Inc."},
		{name: "mac/chrome", os: "mac", browser: "chrome", wantUA: stealth.ChromeUserAgent(stealth.PlatformMacOS, reduced), wantPlatform: "MacIntel", wantVendor: "Google Inc."},
		{name: "mac/safari", os: "mac", browser: "safari", wantUA: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", wantPlatform: "MacIntel", wantVendor: "Apple Computer, Inc."},
		{name: "linux/chrome", os: "linux", browser: "chrome", wantUA: stealth.ChromeUserAgent(stealth.PlatformLinux, reduced), wantPlatform: "Linux x86_64", wantVendor: "Google Inc."},
		{name: "empty browser defaults to chrome", os: "linux", browser: "", wantUA: stealth.ChromeUserAgent(stealth.PlatformLinux, reduced), wantPlatform: "Linux x86_64", wantVendor: "Google Inc."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp, err := h.generateFingerprint(fingerprintRequest{OS: tc.os, Browser: tc.browser})
			if err != nil {
				t.Fatalf("generateFingerprint: %v", err)
			}
			if fp.UserAgent != tc.wantUA {
				t.Errorf("userAgent =\n  %q\nwant\n  %q", fp.UserAgent, tc.wantUA)
			}
			if fp.Platform != tc.wantPlatform {
				t.Errorf("platform = %q, want %q", fp.Platform, tc.wantPlatform)
			}
			if fp.Vendor != tc.wantVendor {
				t.Errorf("vendor = %q, want %q", fp.Vendor, tc.wantVendor)
			}
		})
	}
}

// Scoped to the default browser deliberately — chrome is the one browser both
// weighted os rows hold, so this arm cannot fail whatever the selection does. The
// two tests below carry the non-chrome cases. Run enough times to see both arms of
// the weighting.
func TestRandomOSNeverReachesTheRefusalPath(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}

	seen := map[string]bool{}
	for range 200 {
		fp, err := h.generateFingerprint(fingerprintRequest{OS: "random"})
		if err != nil {
			t.Fatalf("os=random was refused: %v", err)
		}
		if fp.UserAgent == "" {
			t.Fatal("os=random produced an empty userAgent")
		}
		seen[fp.Platform] = true
	}
	for _, want := range []string{"Win32", "MacIntel"} {
		if !seen[want] {
			t.Errorf("os=random never resolved to %s in 200 draws; the weighting or the matrix changed", want)
		}
	}
}

// os: "random" resolves only to an os whose row holds the requested browser. Picking
// an os first and looking the pair up second made the same request body answer 200 or
// 400 by coin flip: safari refused whenever the pick was windows. Asserted on every
// iteration, because the defect it replaces was intermittent rather than absent.
func TestRandomOSResolvesToAnOSThatHoldsTheRequestedBrowser(t *testing.T) {
	const version = "144.0.7559.133"
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: version}}
	reduced := stealth.ReducedBrowserVersion(version)
	windowsChrome := stealth.ChromeUserAgent(stealth.PlatformWindows, reduced)
	matrix := h.fingerprintMatrix()

	for _, tc := range []struct {
		browser      string
		wantUA       string
		wantPlatform string
	}{
		{browser: "safari", wantUA: matrix["mac"]["safari"].UserAgent, wantPlatform: "MacIntel"},
		{browser: "edge", wantUA: stealth.EdgeUserAgent(windowsChrome, reduced), wantPlatform: "Win32"},
	} {
		t.Run(tc.browser, func(t *testing.T) {
			for i := range 200 {
				fp, err := h.generateFingerprint(fingerprintRequest{OS: "random", Browser: tc.browser})
				if err != nil {
					t.Fatalf("draw %d: os=random browser=%s was refused: %v", i, tc.browser, err)
				}
				if fp.UserAgent != tc.wantUA {
					t.Fatalf("draw %d: userAgent =\n  %q\nwant\n  %q", i, fp.UserAgent, tc.wantUA)
				}
				if fp.Platform != tc.wantPlatform {
					t.Fatalf("draw %d: platform = %q, want %q", i, fp.Platform, tc.wantPlatform)
				}
			}
		})
	}
}

// Narrowing the random pick to the rows that hold the browser must not remove the
// refusal this card exists to add: a browser no weighted row holds is still refused,
// and the message names what the caller SENT. Being told windows/firefox is
// unavailable after asking for os: "random" is not actionable — they never asked for
// windows, and retrying may well succeed.
func TestRandomOSStillRefusesABrowserNoRowHolds(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}

	for i := range 200 {
		fp, err := h.generateFingerprint(fingerprintRequest{OS: "random", Browser: "firefox"})
		if err == nil {
			t.Fatalf("draw %d: os=random browser=firefox was accepted with userAgent %q", i, fp.UserAgent)
		}
		if fp != (fingerprint{}) {
			t.Fatalf("draw %d: refused request returned a populated fingerprint: %+v", i, fp)
		}
		for _, want := range []string{`"firefox"`, `"random"`} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not name the requested %s", err, want)
			}
		}
		for _, resolved := range []string{`os "windows"`, `os "mac"`} {
			if strings.Contains(err.Error(), resolved) {
				t.Fatalf("error %q reports a resolved os the caller never supplied", err)
			}
		}
		for _, want := range availableFingerprintPairs(h.fingerprintMatrix()) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q omits the available pair %q", err, want)
			}
		}
	}
}

// The weighting over the constrained candidates must stay the weighting: chrome is
// held by both rows, so it keeps the 0.7/0.3 split rather than collapsing to one os.
func TestRandomOSKeepsBothArmsForABrowserBothRowsHold(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}

	seen := map[string]int{}
	for range 400 {
		fp, err := h.generateFingerprint(fingerprintRequest{OS: "random", Browser: "chrome"})
		if err != nil {
			t.Fatalf("os=random browser=chrome was refused: %v", err)
		}
		seen[fp.Platform]++
	}
	for _, want := range []string{"Win32", "MacIntel"} {
		if seen[want] == 0 {
			t.Fatalf("os=random browser=chrome never resolved to %s in 400 draws: %v", want, seen)
		}
	}
	if seen["Win32"] <= seen["MacIntel"] {
		t.Errorf("windows is weighted 0.7 against mac 0.3 but drew %v", seen)
	}
}

// The endpoint contract, not just the helper: an unlisted pair must be a 400 with a
// machine-readable code, and must never be a 200 carrying an empty userAgent. The
// refusal also has to happen before the browser is touched — a rejected request must
// not leave a half-applied identity on the tab.
func TestHandleFingerprintRotateRefusesAnUnlistedPair(t *testing.T) {
	for _, body := range []string{
		`{"os":"linux","browser":"edge"}`,
		`{"os":"mac","browser":"edge"}`,
		`{"os":"windows","browser":"safari"}`,
		`{"os":"bogus"}`,
		`{"os":"random","browser":"firefox"}`,
	} {
		t.Run(body, func(t *testing.T) {
			mb := &mockBridge{}
			h := New(mb, &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}, nil, nil, nil)
			req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(body)))
			w := httptest.NewRecorder()

			h.HandleFingerprintRotate(w, req)

			if w.Code != 400 {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			var resp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("parse body: %v", err)
			}
			if resp["code"] != "unsupported_fingerprint" {
				t.Errorf("code = %v, want unsupported_fingerprint", resp["code"])
			}
			message, _ := resp["error"].(string)
			for _, want := range availableFingerprintPairs(h.fingerprintMatrix()) {
				if !strings.Contains(message, want) {
					t.Errorf("error %q omits the available pair %q", message, want)
				}
			}
			if _, ok := resp["fingerprint"]; ok {
				t.Errorf("a refusal returned a fingerprint payload: %s", w.Body.String())
			}
		})
	}
}

// A listed pair still rotates, so the refusal did not shadow the success path.
func TestHandleFingerprintRotateStillAcceptsAListedPair(t *testing.T) {
	mb := &mockBridge{}
	h := New(mb, &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(`{"os":"linux","browser":"chrome"}`)))
	w := httptest.NewRecorder()

	h.HandleFingerprintRotate(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	fp, ok := resp["fingerprint"].(map[string]any)
	if !ok {
		t.Fatalf("no fingerprint in response: %s", w.Body.String())
	}
	if ua, _ := fp["userAgent"].(string); ua == "" {
		t.Error("a rotated fingerprint carries an empty userAgent")
	}
}

// probeMatrix is the real matrix plus one browser held only by unweighted rows —
// the state no shipped matrix reaches yet, and the one the weighted-only draw got
// wrong. fingerprintMatrix returns a fresh literal per call, so mutating the copy
// cannot leak into another test.
func probeMatrix(t *testing.T, h *Handlers, browser string, osNames ...string) map[string]map[string]fingerprint {
	t.Helper()
	matrix := h.fingerprintMatrix()
	for _, osName := range osNames {
		for _, weighted := range randomFingerprintOSWeights {
			if osName == weighted.name {
				t.Fatalf("probe os %q is in randomFingerprintOSWeights, so it cannot stand in for an unweighted row", osName)
			}
		}
		if matrix[osName] == nil {
			matrix[osName] = map[string]fingerprint{}
		}
		matrix[osName][browser] = fingerprint{UserAgent: "probe-" + osName + "-" + browser}
	}
	for _, weighted := range randomFingerprintOSWeights {
		if _, ok := matrix[weighted.name][browser]; ok {
			t.Fatalf("probe browser %q is held by the weighted row %q, so the fallback under test is never reached", browser, weighted.name)
		}
	}
	return matrix
}

// The gap this card closes: a browser held only by an unweighted row was refused
// under os: "random" while the refusal listed that very pair as available. The
// caller delegated the os choice and named a browser the product has, so a refusal
// tells them nothing they can act on. Asserted on every draw, because the defect it
// replaces was a coin flip rather than a constant.
func TestRandomOSResolvesABrowserOnlyAnUnweightedRowHolds(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	matrix := probeMatrix(t, &h, "firefox", "linux")

	for i := range 200 {
		picked, ok := resolveRandomFingerprintOS(matrix, "firefox")
		if !ok {
			t.Fatalf("draw %d: os=random refused firefox although linux/firefox exists; the refusal would list a pair the caller cannot reach", i)
		}
		if picked != "linux" {
			t.Fatalf("draw %d: os=random resolved to %q, want linux — the only row holding firefox", i, picked)
		}
	}
}

// With several unweighted rows holding the browser, the pick must be uniform and
// must not depend on map iteration order — a silently order-dependent fallback is
// the next defect of this shape.
func TestRandomOSFallbackPicksUniformlyAmongUnweightedRows(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	matrix := probeMatrix(t, &h, "firefox", "linux", "freebsd")

	const draws = 1200
	seen := map[string]int{}
	for i := range draws {
		picked, ok := resolveRandomFingerprintOS(matrix, "firefox")
		if !ok {
			t.Fatalf("draw %d: firefox refused although two rows hold it", i)
		}
		seen[picked]++
	}
	for _, want := range []string{"linux", "freebsd"} {
		if seen[want] == 0 {
			t.Fatalf("%q never drawn in %d draws (%v); the fallback picks one row rather than among them", want, draws, seen)
		}
		// A wide band: this pins uniform-ish against first-wins or order-dependent,
		// not the RNG's quality.
		if share := float64(seen[want]) / draws; share < 0.35 || share > 0.65 {
			t.Errorf("%q drawn %.0f%% of %d draws (%v), want roughly half each", want, share*100, draws, seen)
		}
	}
}

// The fallback is reached ONLY when no weighted row holds the browser. This is what
// keeps the 0.7/0.3 split and the default request untouched, and it is the half a
// widening change is most likely to overshoot: chrome is held by weighted rows, so
// an unweighted row holding it too must never be drawn.
func TestRandomOSIgnoresUnweightedRowsWhenAWeightedRowHoldsTheBrowser(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	matrix := h.fingerprintMatrix()
	matrix["linux"]["edge"] = fingerprint{UserAgent: "probe-linux-edge"}

	for i := range 400 {
		for _, browser := range []string{"chrome", "edge"} {
			picked, ok := resolveRandomFingerprintOS(matrix, browser)
			if !ok {
				t.Fatalf("draw %d: %s refused", i, browser)
			}
			if picked == "linux" {
				t.Fatalf("draw %d: os=random browser=%s resolved to linux, which is unweighted; the weighted rows hold it, so the fallback must not be reached", i, browser)
			}
		}
	}
}

// The candidate list is what the draw runs over, and it must not vary with map
// iteration order: same matrix, same list, every time. Built repeatedly because Go
// randomises range order per loop, so a single call cannot tell a sorted list from
// a lucky one.
func TestFingerprintOSCandidatesAreStableAndEquallyWeightedInTheFallback(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	matrix := probeMatrix(t, &h, "firefox", "linux", "freebsd", "openbsd")
	want := []string{"freebsd", "linux", "openbsd"}

	for i := range 50 {
		candidates := fingerprintOSCandidates(matrix, "firefox")
		got := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			got = append(got, candidate.name)
			if candidate.weight != 1 {
				t.Fatalf("fallback candidate %q carries weight %v, want 1 — the fallback is uniform, and a weight here is a second place the 0.7/0.3 split could be edited", candidate.name, candidate.weight)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("build %d: candidates = %v, want %v every time; the order follows map iteration, so the pick is not reproducible", i, got, want)
		}
	}

	// The weighted arm keeps its own weights and is not sorted into the fallback's shape.
	for _, candidate := range fingerprintOSCandidates(matrix, "chrome") {
		if candidate.weight == 1 {
			t.Errorf("weighted candidate %q lost its weight (%v); chrome must keep the 0.7/0.3 split", candidate.name, candidate.weight)
		}
	}
}

// What makes the refusal's full-matrix listing honest after the widening: os:
// "random" refuses only a browser NO row holds, so a refusal never names a pair the
// caller could have had. Derived from the matrix rather than from a hand-picked
// pair or two, so the next row added is covered without editing this test — which
// is the case this card exists for.
func TestEveryBrowserTheMatrixHoldsIsReachableThroughRandom(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	matrix := h.fingerprintMatrix()

	holders := map[string][]string{}
	for osName, row := range matrix {
		for browser := range row {
			holders[browser] = append(holders[browser], osName)
		}
	}
	browsers := map[string]bool{}
	for browser, rows := range holders {
		sort.Strings(rows)
		holders[browser] = rows
		browsers[browser] = true
	}
	if len(browsers) == 0 {
		t.Fatal("the matrix holds no browsers, so this guard would pass vacuously")
	}

	for browser := range browsers {
		t.Run(browser, func(t *testing.T) {
			for i := range 100 {
				if _, ok := resolveRandomFingerprintOS(matrix, browser); !ok {
					t.Fatalf("draw %d: os=random refused %q although %v hold it; the refusal would list those pairs as available while refusing the browser in them",
						i, browser, holders[browser])
				}
				fp, err := h.generateFingerprint(fingerprintRequest{OS: "random", Browser: browser})
				if err != nil {
					t.Fatalf("draw %d: os=random browser=%s refused end to end: %v", i, browser, err)
				}
				if fp.UserAgent == "" {
					t.Fatalf("draw %d: os=random browser=%s resolved to an empty identity", i, browser)
				}
			}
		})
	}
}

// The simplest possible call, and the one an agent or a curl writes first. It named
// no os, so there is nothing to contradict by answering with this host's identity —
// and the launch persona already resolves from the host, so this is also what makes
// the endpoint and the browser PinchTab launches agree. Refusing it was PIN-121's
// rule applied one input too far.
func TestGenerateFingerprintWithNoOSAnswersWithThisHostsIdentity(t *testing.T) {
	const version = "144.0.7559.133"
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: version}}
	persona := stealth.BuildPersona("", version)

	fp, err := h.generateFingerprint(fingerprintRequest{})
	if err != nil {
		t.Fatalf("the bare request was refused: %v", err)
	}
	if fp.UserAgent != persona.UserAgent {
		t.Fatalf("bare request userAgent =\n  %q\nwant this host's persona\n  %q", fp.UserAgent, persona.UserAgent)
	}
	if fp.Platform != persona.NavigatorPlatform {
		t.Errorf("bare request platform = %q, want the persona's %q", fp.Platform, persona.NavigatorPlatform)
	}
	if fp.UserAgent == "" {
		t.Error("the bare request produced an empty userAgent, which is the blank identity this card exists to stop")
	}
}

// identityOf is the part of a fingerprint that the os/browser selection decides.
// CPUCores, Memory and a random screen are drawn per call, so comparing whole
// structs would compare the dice.
func identityOf(fp fingerprint) [3]string {
	return [3]string{fp.UserAgent, fp.Platform, fp.Vendor}
}

// Two vocabularies for the same three platforms: internal/stealth spells them
// Windows/macOS/Linux, the matrix keys them windows/mac/linux. "macOS" is the one
// that dies under case folding — and it is the spelling a caller who has read
// stealth.PlatformMacOS will send — so every member is asserted, both spellings and
// mixed case, against the fingerprint the canonical key returns.
func TestEverySpellingOfAPlatformResolvesToTheSameFingerprint(t *testing.T) {
	const version = "144.0.7559.133"
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: version}}

	for _, tc := range []struct{ key, spelling string }{
		{key: stealth.FingerprintOSWindows, spelling: stealth.PlatformWindows},
		{key: stealth.FingerprintOSWindows, spelling: "WINDOWS"},
		{key: stealth.FingerprintOSMac, spelling: stealth.PlatformMacOS},
		{key: stealth.FingerprintOSMac, spelling: "MacOS"},
		{key: stealth.FingerprintOSLinux, spelling: stealth.PlatformLinux},
		{key: stealth.FingerprintOSLinux, spelling: " linux "},
	} {
		t.Run(tc.spelling, func(t *testing.T) {
			want, err := h.generateFingerprint(fingerprintRequest{OS: tc.key})
			if err != nil {
				t.Fatalf("os=%q was refused: %v", tc.key, err)
			}
			got, err := h.generateFingerprint(fingerprintRequest{OS: tc.spelling})
			if err != nil {
				t.Fatalf("os=%q was refused although it is the same platform as %q: %v", tc.spelling, tc.key, err)
			}
			if identityOf(got) != identityOf(want) {
				t.Errorf("os=%q returned\n  %v\nwant the same identity as os=%q\n  %v", tc.spelling, identityOf(got), tc.key, identityOf(want))
			}
		})
	}

	// The browser name folds too, and an absent one still defaults to chrome.
	lower, err := h.generateFingerprint(fingerprintRequest{OS: stealth.FingerprintOSWindows, Browser: "edge"})
	if err != nil {
		t.Fatalf("browser=edge was refused: %v", err)
	}
	for _, spelling := range []string{"Edge", "EDGE", " edge "} {
		got, err := h.generateFingerprint(fingerprintRequest{OS: stealth.FingerprintOSWindows, Browser: spelling})
		if err != nil {
			t.Fatalf("browser=%q was refused: %v", spelling, err)
		}
		if identityOf(got) != identityOf(lower) {
			t.Errorf("browser=%q returned %v, want the same identity as browser=edge %v", spelling, identityOf(got), identityOf(lower))
		}
	}
}

// The host default must not become a catch-all: an os that is not a platform at all
// is still a miss, and PIN-121's refusal is what a caller needs to see. Asserted
// alongside an unlisted pair, because the two refusals share the message.
func TestAnUnknownOSStillRefusesRatherThanFallingBackToTheHost(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	pairs := availableFingerprintPairs(h.fingerprintMatrix())

	for _, tc := range []struct {
		name string
		req  fingerprintRequest
	}{
		{name: "unknown os", req: fingerprintRequest{OS: "ubuntu"}},
		{name: "unknown os with a known browser", req: fingerprintRequest{OS: "ubuntu", Browser: "chrome"}},
		{name: "unlisted pair", req: fingerprintRequest{OS: stealth.FingerprintOSLinux, Browser: "edge"}},
		{name: "unknown browser", req: fingerprintRequest{OS: stealth.FingerprintOSWindows, Browser: "firefox"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fp, err := h.generateFingerprint(tc.req)
			if err == nil {
				t.Fatalf("%+v was accepted with userAgent %q; the host default must not swallow a genuine miss", tc.req, fp.UserAgent)
			}
			if fp != (fingerprint{}) {
				t.Errorf("refused request returned a populated fingerprint: %+v", fp)
			}
			for _, want := range pairs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q omits the available pair %q", err, want)
				}
			}
		})
	}

	// The message names what the caller sent, not a translated or defaulted value.
	_, err := h.generateFingerprint(fingerprintRequest{OS: "ubuntu"})
	if err == nil {
		t.Fatal("os=ubuntu was accepted")
	}
	if !strings.Contains(err.Error(), `"ubuntu"`) {
		t.Errorf("error %q does not name the os the caller sent", err)
	}
	if strings.Contains(err.Error(), stealth.HostFingerprintOS()+`" with`) {
		t.Errorf("error %q reports the host default instead of the requested os", err)
	}
}

// The refusal is only actionable if it names the os in the words the caller used.
// Resolving translates the spelling before the pair lookup, so a miss reported after
// the translation quotes an internal key the caller never sent — and a caller who
// named no os at all would be quoted an empty string.
func TestTheRefusalNamesTheOSTheCallerSent(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}

	for _, spelling := range []string{stealth.PlatformMacOS, stealth.FingerprintOSMac} {
		t.Run(spelling, func(t *testing.T) {
			_, err := h.generateFingerprint(fingerprintRequest{OS: spelling, Browser: "edge"})
			if err == nil {
				t.Fatalf("os=%q browser=edge was accepted", spelling)
			}
			if !strings.Contains(err.Error(), `os "`+spelling+`"`) {
				t.Errorf("error %q does not name the os the caller sent (%q)", err, spelling)
			}
		})
	}

	_, err := h.generateFingerprint(fingerprintRequest{Browser: "firefox"})
	if err == nil {
		t.Fatal("browser=firefox was accepted with no os")
	}
	if strings.Contains(err.Error(), `os ""`) {
		t.Errorf("error %q quotes an empty os to a caller who named none", err)
	}
	if !strings.Contains(err.Error(), `the host os "`+stealth.HostFingerprintOS()+`"`) {
		t.Errorf("error %q does not say the host os was used, nor name it", err)
	}
}

// The two vocabularies must stay in step: every platform internal/stealth knows maps
// to a key the matrix holds, and every os key the matrix holds is one internal/stealth
// owns. Derived from both sides, so adding a row to either without the other reds.
func TestThePlatformAndMatrixVocabulariesAgree(t *testing.T) {
	h := Handlers{Config: &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}}
	matrix := h.fingerprintMatrix()

	platforms := stealth.FingerprintPlatforms()
	if len(platforms) < len(matrix) {
		t.Fatalf("FingerprintPlatforms() returned %d platforms for a matrix of %d rows; the census cannot cover the vocabulary", len(platforms), len(matrix))
	}

	owned := map[string]bool{}
	for _, platform := range platforms {
		key, ok := stealth.FingerprintOSKey(platform)
		if !ok {
			t.Fatalf("internal/stealth knows platform %q but maps it to no fingerprint os key", platform)
		}
		owned[key] = true
		if _, held := matrix[key]; !held {
			t.Errorf("platform %q maps to os key %q, which the matrix does not hold — a request naming that platform would refuse", platform, key)
		}
	}
	for key := range matrix {
		if !owned[key] {
			t.Errorf("the matrix holds os key %q that no platform in internal/stealth maps to, so no persona spelling can reach it", key)
		}
	}
	if got := stealth.HostFingerprintOS(); !owned[got] {
		t.Fatalf("HostFingerprintOS() = %q, which is not one of the keys it owns", got)
	}
}

// rotateOrderBridge records the override calls a rotation makes, in the order it makes
// them, and can fail the locale override the way Chrome intermittently does.
type rotateOrderBridge struct {
	*mockBridge
	calls     []string
	localeErr error
}

func (m *rotateOrderBridge) SetUserAgentOverride(ctx context.Context, p bridge.UserAgentOverrideParams) error {
	m.calls = append(m.calls, "userAgent")
	return nil
}

func (m *rotateOrderBridge) SetLocaleOverride(ctx context.Context, locale string) error {
	m.calls = append(m.calls, "locale")
	return m.localeErr
}

func rotate(t *testing.T, mb *rotateOrderBridge, body string) *httptest.ResponseRecorder {
	t.Helper()

	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	w := httptest.NewRecorder()
	h.HandleFingerprintRotate(w, httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(body))))
	return w
}

// The locale override is the one that fails — Chrome answers "Another locale override is
// already in effect" often enough to matter — so it goes first. Applying the UA before it
// left the tab wearing half a new identity, with no rollback and no correct meaning.
func TestRotateAppliesTheFailureProneLocaleOverrideBeforeTheUserAgent(t *testing.T) {
	mb := &rotateOrderBridge{mockBridge: &mockBridge{}}

	if w := rotate(t, mb, `{"os":"windows","browser":"edge","tabId":"tab1"}`); w.Code != 200 {
		t.Fatalf("rotate returned %d: %s", w.Code, w.Body.String())
	}

	if len(mb.calls) < 2 || mb.calls[0] != "locale" || mb.calls[1] != "userAgent" {
		t.Fatalf("rotate applied %v, want the locale override before the user agent", mb.calls)
	}
}

func TestAFailedLocaleOverrideLeavesTheUserAgentUntouched(t *testing.T) {
	mb := &rotateOrderBridge{mockBridge: &mockBridge{}, localeErr: errors.New("Another locale override is already in effect (-32000)")}

	w := rotate(t, mb, `{"os":"windows","browser":"edge","tabId":"tab1"}`)

	if w.Code != 500 {
		t.Fatalf("rotate returned %d, want 500 when the locale override fails", w.Code)
	}
	for _, call := range mb.calls {
		if call == "userAgent" {
			t.Fatalf("rotate applied %v after the locale override failed, so the tab is half-rotated with no way back", mb.calls)
		}
	}
}
