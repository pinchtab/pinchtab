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

func TestDefaultChromeFlagArgsDisablesMetricsReporting(t *testing.T) {
	args := defaultChromeFlagArgs()
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

func TestDefaultChromeFlagArgsPreservesPopupBlockingAndSiteIsolation(t *testing.T) {
	args := defaultChromeFlagArgs()
	for _, forbidden := range []string{
		"--disable-popup-blocking",
		"--no-sandbox",
		"--disable-features=site-per-process,Translate,BlinkGenPropertyTrees",
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
