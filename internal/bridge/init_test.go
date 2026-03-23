package bridge

import (
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestBuildChromeArgsSuppressesCrashDialogs(t *testing.T) {
	args := buildChromeArgs(&config.RuntimeConfig{}, 9222)

	for _, want := range []string{
		"--disable-session-crashed-bubble",
		"--hide-crash-restore-bubble",
		"--noerrdialogs",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing chrome arg %q in %v", want, args)
		}
	}
}

func TestBuildChromeArgsIncludesStealthLaunchFlags(t *testing.T) {
	args := buildChromeArgs(&config.RuntimeConfig{}, 9222)

	for _, want := range []string{
		"--enable-automation=false",
		"--enable-network-information-downlink-max",
		"--disable-blink-features=AutomationControlled",
		"--lang=en-US",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing chrome arg %q in %v", want, args)
		}
	}
}

func TestBuildChromeArgsHeadlessUsesSoftwareRendering(t *testing.T) {
	args := buildChromeArgs(&config.RuntimeConfig{Headless: true}, 9222)

	for _, want := range []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-vulkan",
		"--use-angle=swiftshader",
		"--enable-unsafe-swiftshader",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing headless chrome arg %q in %v", want, args)
		}
	}
}

func TestBuildChromeArgsIncludesGlobalUserAgent(t *testing.T) {
	args := buildChromeArgs(&config.RuntimeConfig{ChromeVersion: "144.0.7559.133"}, 9222)

	found := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--user-agent=Mozilla/5.0") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected global user-agent arg in %v", args)
	}
}

func TestBuildChromeArgsSanitizesUnsafeAndReservedExtraFlags(t *testing.T) {
	args := buildChromeArgs(&config.RuntimeConfig{
		ChromeVersion:    "144.0.7559.133",
		ChromeExtraFlags: "--disable-gpu --user-agent=Bad/1.0 --disable-web-security --ash-no-nudges",
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

func TestBaseChromeFlagArgsDisablesMetricsReporting(t *testing.T) {
	args := baseChromeFlagArgs()
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

func TestBaseChromeFlagArgsPreservesPopupBlockingAndSiteIsolation(t *testing.T) {
	args := baseChromeFlagArgs()
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
