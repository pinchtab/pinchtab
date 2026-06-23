package bridge

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestBuildBrowserArgsSuppressesCrashDialogs(t *testing.T) {
	args := buildBrowserArgs(&config.RuntimeConfig{}, 9222)

	for _, want := range []string{
		"--disable-session-crashed-bubble",
		"--hide-crash-restore-bubble",
		"--noerrdialogs",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing browser arg %q in %v", want, args)
		}
	}
}

func TestBuildBrowserArgsIncludesStealthLaunchFlags(t *testing.T) {
	args := buildBrowserArgs(&config.RuntimeConfig{}, 9222)

	for _, want := range []string{
		"--enable-automation=false",
		"--enable-network-information-downlink-max",
		"--disable-blink-features=AutomationControlled",
		"--lang=en-US",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing browser arg %q in %v", want, args)
		}
	}
}

func TestBuildBrowserArgsHeadlessUsesSoftwareRendering(t *testing.T) {
	args := buildBrowserArgs(&config.RuntimeConfig{Headless: true}, 9222)

	for _, want := range []string{
		"--headless=new",
		"--disable-vulkan",
		"--use-angle=swiftshader",
		"--enable-unsafe-swiftshader",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing headless browser arg %q in %v", want, args)
		}
	}
	// --disable-gpu would remove the compositor backend that the swiftshader
	// flags above provide; capture/print CDP calls then hang.
	if slices.Contains(args, "--disable-gpu") {
		t.Fatalf("headless args must not contain --disable-gpu: %v", args)
	}
}

func TestBuildBrowserArgsIncludesGlobalUserAgent(t *testing.T) {
	// HEADED + no custom UA: Chrome must run WITHOUT --user-agent so its
	// native, complete high-entropy UA Client Hints are served.
	args := buildBrowserArgs(&config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}, 9222)
	for _, arg := range args {
		if strings.HasPrefix(arg, "--user-agent=") {
			t.Fatalf("did not expect a pinned user-agent in headed mode without a custom UA, got %v", args)
		}
	}

	// HEADLESS + no custom UA: Chrome's NATIVE userAgent contains
	// "HeadlessChrome/..." in --headless=new. We MUST pin --user-agent so the
	// headless tell never reaches the page or workers. The native UA-CH is
	// already degraded in headless, so the PR #580 UA-CH realism rationale
	// does not apply here.
	headless := buildBrowserArgs(&config.RuntimeConfig{BrowserVersion: "144.0.7559.133", Headless: true}, 9222)
	var headlessUA string
	for _, arg := range headless {
		if strings.HasPrefix(arg, "--user-agent=") {
			headlessUA = strings.TrimPrefix(arg, "--user-agent=")
			break
		}
	}
	if headlessUA == "" {
		t.Fatalf("expected --user-agent to be pinned in headless mode (HeadlessChrome would otherwise leak in navigator.userAgent), got %v", headless)
	}
	if strings.Contains(headlessUA, "HeadlessChrome") {
		t.Fatalf("pinned headless UA must not contain HeadlessChrome, got %q", headlessUA)
	}
	if !strings.Contains(headlessUA, "Chrome/144.0.0.0") {
		t.Fatalf("pinned headless UA should carry the configured frozen Chrome major, got %q", headlessUA)
	}

	// Explicit custom UA: it must be pinned (independent of headless).
	custom := buildBrowserArgs(&config.RuntimeConfig{BrowserVersion: "144.0.7559.133", UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"}, 9222)
	found := false
	for _, arg := range custom {
		if strings.HasPrefix(arg, "--user-agent=Mozilla/5.0") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an explicit custom UA to be pinned in %v", custom)
	}
}

func TestBuildBrowserArgsSanitizesUnsafeAndReservedExtraFlags(t *testing.T) {
	args := buildBrowserArgs(&config.RuntimeConfig{
		BrowserVersion:    "144.0.7559.133",
		BrowserExtraFlags: "--disable-gpu --user-agent=Bad/1.0 --disable-web-security --ash-no-nudges",
	}, 9222)

	if !slices.Contains(args, "--disable-gpu") {
		t.Fatalf("expected safe extra flag to be preserved in %v", args)
	}
	if !slices.Contains(args, "--ash-no-nudges") {
		t.Fatalf("expected safe extra flag to be preserved in %v", args)
	}
	for _, forbidden := range []string{"--user-agent=Bad/1.0", "--disable-web-security"} {
		if slices.Contains(args, forbidden) {
			t.Fatalf("did not expect forbidden extra flag %q in %v", forbidden, args)
		}
	}
}

func TestBuildBrowserArgsSkipsMissingExtensionPaths(t *testing.T) {
	args := buildBrowserArgs(&config.RuntimeConfig{
		ExtensionPaths: []string{filepath.Join(t.TempDir(), "missing-extension")},
	}, 9222)

	if !slices.Contains(args, "--disable-extensions") {
		t.Fatalf("expected missing extension paths to fall back to --disable-extensions, got %v", args)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--load-extension=") {
			t.Fatalf("did not expect load-extension arg for missing path: %v", args)
		}
	}
}

func TestBuildBrowserArgsIncludesExistingExtensionPaths(t *testing.T) {
	extensionDir := filepath.Join(t.TempDir(), "extensions", "example")
	if err := os.MkdirAll(extensionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	args := buildBrowserArgs(&config.RuntimeConfig{
		ExtensionPaths: []string{extensionDir},
	}, 9222)

	found := false
	for _, arg := range args {
		if arg == "--load-extension="+extensionDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected load-extension arg for existing path in %v", args)
	}
}

func TestBuildBrowserArgsIncludesProxyFlagsWhenConfigured(t *testing.T) {
	args := buildBrowserArgs(&config.RuntimeConfig{
		Proxy: config.BrowserProxyConfig{
			Server:     "http://proxy.example.com:8080",
			BypassList: []string{"*.local", "127.0.0.1"},
			Username:   "alice",
			Password:   "s3cr3t",
		},
	}, 9222)

	hasProxy, hasBypass := false, false
	for _, a := range args {
		if a == "--proxy-server=http://proxy.example.com:8080" {
			hasProxy = true
		}
		if a == "--proxy-bypass-list=*.local;127.0.0.1" {
			hasBypass = true
		}
		if strings.Contains(a, "alice") || strings.Contains(a, "s3cr3t") {
			t.Fatalf("proxy credentials leaked into browser args: %q", a)
		}
	}
	if !hasProxy {
		t.Fatalf("expected --proxy-server flag in %v", args)
	}
	if !hasBypass {
		t.Fatalf("expected --proxy-bypass-list flag in %v", args)
	}
}

func TestBuildBrowserArgsOmitsProxyFlagsWhenDisabled(t *testing.T) {
	args := buildBrowserArgs(&config.RuntimeConfig{}, 9222)
	for _, a := range args {
		if strings.HasPrefix(a, "--proxy-server=") || strings.HasPrefix(a, "--proxy-bypass-list=") {
			t.Fatalf("did not expect proxy flag without config: %q", a)
		}
	}
}

func TestBaseBrowserFlagArgsDisablesMetricsReporting(t *testing.T) {
	args := baseBrowserFlagArgs()
	for _, want := range []string{"--disable-metrics-reporting", "--metrics-recording-only"} {
		found := false
		for _, arg := range args {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %s in args, got %v", want, args)
		}
	}
}

func TestBaseBrowserFlagArgsPreservesPopupBlockingAndSiteIsolation(t *testing.T) {
	args := baseBrowserFlagArgs()
	for _, forbidden := range []string{
		"--disable-popup-blocking",
		"--no-sandbox",
		"--disable-features=site-per-process,Translate,BlinkGenPropertyTrees",
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
	} {
		if slices.Contains(args, forbidden) {
			t.Fatalf("did not expect %s in args: %v", forbidden, args)
		}
	}

	if !slices.Contains(args, "--disable-features=Translate,BlinkGenPropertyTrees") {
		t.Fatalf("expected default disable-features arg to keep non-isolation tweaks, got %v", args)
	}
}

func TestPopupGuardInitScriptNeutralizesOpener(t *testing.T) {
	for _, want := range []string{"window.open", "noopener", "noreferrer", "window.opener"} {
		if !strings.Contains(popupGuardInitScript, want) {
			t.Fatalf("expected popup guard script to contain %q", want)
		}
	}
}
