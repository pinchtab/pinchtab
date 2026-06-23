package chrome_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

func TestRegistration(t *testing.T) {
	b, ok := browsers.Get("chrome")
	if !ok {
		t.Fatal("expected chrome to be registered via init()")
	}
	if b == nil {
		t.Fatal("expected non-nil browser")
	}
}

func TestID(t *testing.T) {
	b, _ := browsers.Get("chrome")
	if got := b.ID(); got != "chrome" {
		t.Fatalf("ID() = %q, want %q", got, "chrome")
	}
}

func TestDisplayName(t *testing.T) {
	b, _ := browsers.Get("chrome")
	if got := b.DisplayName(); got != "Google Chrome" {
		t.Fatalf("DisplayName() = %q, want %q", got, "Google Chrome")
	}
}

func TestCapabilities(t *testing.T) {
	b, _ := browsers.Get("chrome")
	caps := b.Capabilities()

	want := []browsers.BrowserCapability{
		browsers.CapCDP,
		browsers.CapHeadless,
		browsers.CapPDF,
		browsers.CapExtensions,
		browsers.CapDownloads,
		browsers.CapNetworkInterception,
		browsers.CapEventScreencast,
	}

	for _, c := range want {
		if !caps.Has(c) {
			t.Errorf("expected capability %q to be present", c)
		}
	}

	if caps.Has(browsers.CapNativeStealth) {
		t.Error("chrome should NOT have CapNativeStealth")
	}

	if caps.Len() != len(want) {
		t.Errorf("Len() = %d, want %d", caps.Len(), len(want))
	}
}

func TestSupportsRemoteCDP(t *testing.T) {
	b, _ := browsers.Get("chrome")
	if !b.SupportsRemoteCDP() {
		t.Fatal("expected SupportsRemoteCDP() = true")
	}
}

func TestGeoAlignmentEmpty(t *testing.T) {
	b, _ := browsers.Get("chrome")
	geo := b.GeoAlignment(browsers.GeoConfig{})

	if len(geo.Flags) != 0 {
		t.Errorf("expected no flags, got %v", geo.Flags)
	}
	if len(geo.Env) != 0 {
		t.Errorf("expected no env, got %v", geo.Env)
	}
}

func TestBinaryNamesReturnsExpectedNames(t *testing.T) {
	names := chrome.BinaryNames()

	required := []string{"google-chrome", "chromium-browser", "chrome"}
	for _, want := range required {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("BinaryNames() missing %q; got %v", want, names)
		}
	}

	names[0] = "MUTATED"
	fresh := chrome.BinaryNames()
	if fresh[0] == "MUTATED" {
		t.Fatal("BinaryNames() did not return a defensive copy")
	}
}

func TestCommonPathsPerOS(t *testing.T) {
	tests := []struct {
		goos    string
		want    string // substring that must appear in at least one path
		wantNil bool
	}{
		{"linux", "/usr/bin/google-chrome", false},
		{"darwin", "Google Chrome.app", false},
		{"windows", "chrome.exe", false},
		{"freebsd", "", true},
	}

	for _, tt := range tests {
		paths := chrome.CommonPaths(tt.goos)

		if tt.wantNil {
			if paths != nil {
				t.Errorf("CommonPaths(%q) = %v, want nil", tt.goos, paths)
			}
			continue
		}

		if len(paths) == 0 {
			t.Errorf("CommonPaths(%q) returned empty slice", tt.goos)
			continue
		}

		found := false
		for _, p := range paths {
			if strings.Contains(p, tt.want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CommonPaths(%q) missing path containing %q; got %v", tt.goos, tt.want, paths)
		}
	}
}

func TestDiscoverBinaryReturnsValidResult(t *testing.T) {
	b, ok := browsers.Get("chrome")
	if !ok {
		t.Fatal("chrome not registered")
	}

	d := b.DiscoverBinary()

	// In CI the binary may not be installed, so Found can be empty.
	// But Probed must always be non-empty because we check at least the
	// binary names via PATH.
	if len(d.Probed) == 0 {
		t.Fatal("DiscoverBinary().Probed is empty; expected at least PATH probes")
	}

	_ = browsers.BinaryDiscovery(d)
}

func TestBuildLaunchArgsReturnsBaseFlags(t *testing.T) {
	args, _, err := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{})
	if err != nil {
		t.Fatalf("BuildLaunchArgs() error = %v", err)
	}

	for _, want := range []string{
		"--disable-background-networking",
		"--disable-metrics-reporting",
		"--metrics-recording-only",
		"--password-store=basic",
		"--use-mock-keychain",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("expected args to contain %q", want)
		}
	}

	if len(args) != 25 {
		t.Fatalf("expected 25 args (23 base + 1 --disable-extensions + 1 --window-size), got %d: %v", len(args), args)
	}
}

func TestBuildLaunchArgsReturnsNoEnv(t *testing.T) {
	_, env, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{})
	if env != nil {
		t.Fatalf("expected nil env, got %v", env)
	}
}

func TestBuildLaunchArgsHeadlessAppendsFlags(t *testing.T) {
	args, env, err := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{Headless: true})
	if err != nil {
		t.Fatalf("BuildLaunchArgs() error = %v", err)
	}
	if env != nil {
		t.Fatalf("expected nil env, got %v", env)
	}

	for _, want := range []string{
		"--headless=new",
		"--disable-vulkan",
		"--use-angle=swiftshader",
		"--enable-unsafe-swiftshader",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("expected headless args to contain %q", want)
		}
	}

	// --disable-gpu must NOT appear: under --headless=new it removes the
	// compositor's GPU backend and capture/print CDP calls hang.
	if slices.Contains(args, "--disable-gpu") {
		t.Error("headless args must not contain --disable-gpu")
	}

	if len(args) != 29 {
		t.Fatalf("expected 29 args (23 base + 4 headless + 1 --disable-extensions + 1 --window-size), got %d: %v", len(args), args)
	}
}

func TestBuildLaunchArgsHeadedOmitsHeadlessFlags(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{Headless: false})
	for _, forbidden := range []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-vulkan",
	} {
		if slices.Contains(args, forbidden) {
			t.Errorf("headed mode should not contain %q", forbidden)
		}
	}
}

func TestBuildLaunchArgsDoesNotContainStealthOrProfileFlags(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{})

	forbidden := []string{
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--headless=new",
		"--user-data-dir",
		"--no-sandbox",
		"--enable-network-information-downlink-max",
	}

	for _, f := range forbidden {
		if slices.Contains(args, f) {
			t.Errorf("args should not contain %q", f)
		}
	}
}

func TestBuildLaunchArgsWithDebugPort(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{DebugPort: 9222})
	if len(args) == 0 || args[0] != "--remote-debugging-port=9222" {
		t.Fatalf("expected first arg to be --remote-debugging-port=9222, got %v", args)
	}
}

func TestBuildLaunchArgsWithProfileDir(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{ProfileDir: "/tmp/test-profile"})
	if !slices.Contains(args, "--user-data-dir=/tmp/test-profile") {
		t.Fatalf("expected --user-data-dir=/tmp/test-profile in args")
	}
}

func TestBuildLaunchArgsWithExtensions(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{
		ExtensionPaths: []string{dir1, dir2, "/nonexistent/path"},
	})
	wantLoad := "--load-extension=" + dir1 + "," + dir2
	wantExcept := "--disable-extensions-except=" + dir1 + "," + dir2
	if !slices.Contains(args, wantLoad) {
		t.Fatalf("expected %q in args, got %v", wantLoad, args)
	}
	if !slices.Contains(args, wantExcept) {
		t.Fatalf("expected %q in args, got %v", wantExcept, args)
	}
}

func TestBuildLaunchArgsNoExtensions(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{})
	if !slices.Contains(args, "--disable-extensions") {
		t.Fatalf("expected --disable-extensions when no extension paths given")
	}
}

func TestBuildLaunchArgsWithTimezone(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{Timezone: "America/New_York"})
	if !slices.Contains(args, "--tz=America/New_York") {
		t.Fatalf("expected --tz=America/New_York in args")
	}
}

func TestBuildLaunchArgsIncludesWindowSize(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{})
	found := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--window-size=") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected --window-size= in args")
	}
}

func TestBuildLaunchArgsWithExtraFlags(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{
		ExtraFlags: []string{"--custom-flag=value"},
	})
	if !slices.Contains(args, "--custom-flag=value") {
		t.Fatalf("expected --custom-flag=value in args")
	}
}

func TestBuildLaunchArgsNoSandbox(t *testing.T) {
	args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{NoSandbox: true})
	if !slices.Contains(args, "--no-sandbox") {
		t.Fatalf("expected --no-sandbox when NoSandbox=true")
	}

	argsNoSandbox, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{NoSandbox: false})
	if slices.Contains(argsNoSandbox, "--no-sandbox") {
		t.Fatalf("expected no --no-sandbox when NoSandbox=false")
	}
}

func indexOf(args []string, prefix string) int {
	for i, a := range args {
		if strings.HasPrefix(a, prefix) {
			return i
		}
	}
	return -1
}

func TestBuildLaunchArgsParityWithRepresentativeConfigs(t *testing.T) {
	t.Run("full headless config", func(t *testing.T) {
		extDir := t.TempDir()
		cfg := browsers.LaunchConfig{
			Headless:       true,
			DebugPort:      9222,
			ProfileDir:     "/tmp/profile",
			Timezone:       "America/New_York",
			ExtensionPaths: []string{extDir},
			ExtraFlags:     []string{"--custom=1"},
			NoSandbox:      true,
		}
		args, env, err := chrome.Browser{}.BuildLaunchArgs(cfg)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if env != nil {
			t.Fatalf("expected nil env, got %v", env)
		}

		mustContain := []string{
			"--remote-debugging-port=9222",
			"--disable-background-networking",
			"--disable-metrics-reporting",
			"--password-store=basic",
			"--headless=new",
			"--disable-vulkan",
			"--use-angle=swiftshader",
			"--enable-unsafe-swiftshader",
			"--load-extension=" + extDir,
			"--disable-extensions-except=" + extDir,
			"--user-data-dir=/tmp/profile",
			"--tz=America/New_York",
			"--custom=1",
			"--no-sandbox",
		}
		for _, want := range mustContain {
			if !slices.Contains(args, want) {
				t.Errorf("missing %q in args", want)
			}
		}

		if indexOf(args, "--window-size=") < 0 {
			t.Error("missing --window-size= arg")
		}

		if args[0] != "--remote-debugging-port=9222" {
			t.Errorf("first arg = %q, want --remote-debugging-port=9222", args[0])
		}

		if args[len(args)-1] != "--no-sandbox" {
			t.Errorf("last arg = %q, want --no-sandbox", args[len(args)-1])
		}

		order := []string{
			"--remote-debugging-port=",
			"--disable-background-networking",
			"--headless=new",
			"--load-extension=",
			"--user-data-dir=",
			"--window-size=",
			"--tz=",
			"--custom=1",
			"--no-sandbox",
		}
		prev := -1
		for _, prefix := range order {
			idx := indexOf(args, prefix)
			if idx < 0 {
				t.Fatalf("ordering check: %q not found", prefix)
			}
			if idx <= prev {
				t.Fatalf("ordering violation: %q at %d, expected after index %d", prefix, idx, prev)
			}
			prev = idx
		}
	})

	t.Run("headed minimal", func(t *testing.T) {
		args, _, err := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{})
		if err != nil {
			t.Fatalf("error = %v", err)
		}

		if len(args) != 25 {
			t.Fatalf("expected 25 args (23 base + disable-extensions + window-size), got %d", len(args))
		}

		if !slices.Contains(args, "--disable-background-networking") {
			t.Error("missing base flag --disable-background-networking")
		}
		if !slices.Contains(args, "--disable-extensions") {
			t.Error("missing --disable-extensions")
		}
		if indexOf(args, "--window-size=") < 0 {
			t.Error("missing --window-size=")
		}

		mustNotContain := []string{
			"--headless=new",
			"--remote-debugging-port=",
			"--user-data-dir=",
			"--tz=",
			"--no-sandbox",
		}
		for _, prefix := range mustNotContain {
			if indexOf(args, prefix) >= 0 {
				t.Errorf("headed minimal should not contain %q", prefix)
			}
		}
	})

	t.Run("extensions with missing paths", func(t *testing.T) {
		validDir := t.TempDir()
		args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{
			ExtensionPaths: []string{validDir, "/nonexistent/ext/path"},
		})

		loadIdx := indexOf(args, "--load-extension=")
		if loadIdx < 0 {
			t.Fatal("expected --load-extension= arg")
		}
		if strings.Contains(args[loadIdx], "/nonexistent/ext/path") {
			t.Error("invalid extension path should be filtered out")
		}
		if !strings.Contains(args[loadIdx], validDir) {
			t.Error("valid extension path should be included")
		}
	})

	t.Run("empty extensions list", func(t *testing.T) {
		args, _, _ := chrome.Browser{}.BuildLaunchArgs(browsers.LaunchConfig{
			ExtensionPaths: []string{},
		})

		if !slices.Contains(args, "--disable-extensions") {
			t.Error("expected --disable-extensions with empty list")
		}
		if indexOf(args, "--load-extension=") >= 0 {
			t.Error("should not have --load-extension with empty list")
		}
	})
}

func TestClassifyLaunchErrorAlwaysUnknown(t *testing.T) {
	b, _ := browsers.Get("chrome")
	kind := b.ClassifyLaunchError(browsers.LaunchFailure{
		Err:             context.Canceled,
		Elapsed:         500 * time.Millisecond,
		ParentCanceled:  false,
		BrowserCanceled: true,
	})
	if kind != browsers.LaunchErrorUnknown {
		t.Errorf("Chrome should always return LaunchErrorUnknown; got %v", kind)
	}
}

func TestChrome_CanHandle(t *testing.T) {
	b := &chrome.Browser{}
	result := b.CanHandle(browsers.RequestIntent{Shape: "static-read"})
	if result.Decision != browsers.DecisionHandle {
		t.Errorf("Chrome CanHandle = %q, want %q", result.Decision, browsers.DecisionHandle)
	}
}

func TestChrome_CanHandle_AllShapes(t *testing.T) {
	b := &chrome.Browser{}
	shapes := []string{
		browsers.ShapeStaticRead,
		browsers.ShapeStaticSnapshot,
		browsers.ShapeRenderedRead,
		browsers.ShapeVisual,
		browsers.ShapeInteraction,
		browsers.ShapeSessionState,
		browsers.ShapeNetworkControl,
		browsers.ShapeDownloadUpload,
		"unknown",
		"",
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			got := b.CanHandle(browsers.RequestIntent{Shape: shape})
			if got.Decision != browsers.DecisionHandle {
				t.Errorf("Chrome CanHandle(%q) = %q, want %q", shape, got.Decision, browsers.DecisionHandle)
			}
		})
	}
}

func TestChrome_ValidateTarget(t *testing.T) {
	b := &chrome.Browser{}
	if err := b.ValidateTarget(browsers.TargetConfig{}); err != nil {
		t.Errorf("ValidateTarget(empty) = %v, want nil", err)
	}
	if err := b.ValidateTarget(browsers.TargetConfig{Binary: "/usr/bin/chrome"}); err != nil {
		t.Errorf("ValidateTarget(binary) = %v, want nil", err)
	}
}

func TestChromeHandleDecisionsCheck(t *testing.T) {
	b := &chrome.Browser{}
	checks := b.DoctorChecks(browsers.TargetConfig{})
	var found *browsers.DoctorCheck
	for i := range checks {
		if checks[i].ID == "handle_decisions" {
			found = &checks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("handle_decisions check not found in Chrome DoctorChecks")
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
	b, _ := browsers.Get("chrome")
	_, _, err := b.BuildLaunchArgs(browsers.LaunchConfig{Mode: browsers.LaunchModeLite})
	if err == nil {
		t.Fatal("expected error for lite mode")
	}
	if !strings.Contains(err.Error(), "chrome") || !strings.Contains(err.Error(), "lite") {
		t.Errorf("error should mention provider and mode; got: %v", err)
	}
}
