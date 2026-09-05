package cloak

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

func configuredPresenceCheck(t *testing.T, binary, versionOutput, platform string) browsers.DoctorCheckResult {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("cloakbrowser_present is skipped on windows")
	}
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	contents := []byte("#!/bin/sh\necho '" + versionOutput + "'\n")
	if err := os.WriteFile(binary, contents, 0o755); err != nil {
		t.Fatal(err)
	}

	original := launchAndEvaluate
	launchAndEvaluate = func(_ context.Context, gotBinary string, args []string, _ time.Duration, expression string, value any) (chrome.CDPProbeResult, error) {
		if gotBinary != binary {
			t.Fatalf("binary = %q, want %q", gotBinary, binary)
		}
		if len(args) != 1 || args[0] != "--fingerprint-platform=windows" {
			t.Fatalf("identity args = %v, want controlled Windows fingerprint probe", args)
		}
		if expression != "navigator.platform" {
			t.Fatalf("expression = %q, want navigator.platform", expression)
		}
		result, ok := value.(*string)
		if !ok {
			t.Fatalf("probe result type = %T, want *string", value)
		}
		*result = platform
		return chrome.CDPProbeResult{Port: 9222}, nil
	}
	t.Cleanup(func() { launchAndEvaluate = original })

	return cloakPresenceCheck(context.Background(), &browsers.DoctorEnv{Binary: binary})
}

func TestCloakPresenceAcceptsBehaviorInsteadOfPathNaming(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "chromium-145.0.0.0", "chrome")
	result := configuredPresenceCheck(t, binary, "Chromium 145.0.0.0", "Win32")
	if result.Status != browsers.DoctorPass {
		t.Fatalf("status = %v, want pass: %s", result.Status, result.Detail)
	}
}

func TestCloakPresenceWarnsWhenBehavioralCloakIsDiscoveredButUnconfigured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cloakbrowser_present is skipped on windows")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "cloakbrowser")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'Chromium 145.0.0.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("HOME", t.TempDir())

	original := launchAndEvaluate
	launchAndEvaluate = func(_ context.Context, _ string, _ []string, _ time.Duration, _ string, value any) (chrome.CDPProbeResult, error) {
		*value.(*string) = "Win32"
		return chrome.CDPProbeResult{Port: 9222}, nil
	}
	t.Cleanup(func() { launchAndEvaluate = original })

	result := cloakPresenceCheck(context.Background(), &browsers.DoctorEnv{})
	if result.Status != browsers.DoctorWarn || !strings.Contains(result.Detail, "browser.binary is unset") {
		t.Fatalf("result = %v %q, want verified discovery setup warning", result.Status, result.Detail)
	}
}

func TestCloakPresenceRejectsChromeEvenUnderCloakNamedDirectory(t *testing.T) {
	for _, version := range []string{"145.0.7632.109", "118.0.0"} {
		t.Run(version, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "cloakbrowser", "google-chrome")
			result := configuredPresenceCheck(t, binary, "Google Chrome "+version, "Linux x86_64")
			if result.Status == browsers.DoctorPass {
				t.Fatalf("ordinary Chrome passed cloakbrowser_present: %s", result.Detail)
			}
			if !strings.Contains(result.Detail, "did not exhibit CloakBrowser fingerprint behavior") {
				t.Fatalf("detail = %q, want behavioral identity warning", result.Detail)
			}
		})
	}
}

func TestCloakPresenceAllowsColdStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cloakbrowser_present is skipped on windows")
	}
	binary := filepath.Join(t.TempDir(), "cloakbrowser")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'Chromium 145.0.0.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	original := launchAndEvaluate
	launchAndEvaluate = func(_ context.Context, _ string, _ []string, timeout time.Duration, _ string, value any) (chrome.CDPProbeResult, error) {
		if timeout < 22*time.Second {
			return chrome.CDPProbeResult{}, context.DeadlineExceeded
		}
		*value.(*string) = "Win32"
		return chrome.CDPProbeResult{Port: 9222}, nil
	}
	t.Cleanup(func() { launchAndEvaluate = original })

	result := cloakPresenceCheck(context.Background(), &browsers.DoctorEnv{Binary: binary})
	if result.Status != browsers.DoctorPass {
		t.Fatalf("status = %v, want pass after cold start: %s", result.Status, result.Detail)
	}
}

func TestCDPReachableAllowsColdStart(t *testing.T) {
	original := launchAndProbe
	launchAndProbe = func(_ context.Context, _ string, _ []string, timeout time.Duration) (chrome.CDPProbeResult, error) {
		if timeout < 22*time.Second {
			return chrome.CDPProbeResult{}, context.DeadlineExceeded
		}
		return chrome.CDPProbeResult{Port: 9222}, nil
	}
	t.Cleanup(func() { launchAndProbe = original })

	result := cdpReachableCheck(context.Background(), &browsers.DoctorEnv{Binary: "/cloakbrowser"})
	if result.Status != browsers.DoctorPass {
		t.Fatalf("status = %v, want pass after cold start: %s", result.Status, result.Detail)
	}
}

func TestCDPReachableUsesDiscoveryWithoutABinaryOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("discovery fixture uses a shell script")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, BinaryNames()[0])
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	original := launchAndProbe
	var launched string
	launchAndProbe = func(_ context.Context, path string, _ []string, _ time.Duration) (chrome.CDPProbeResult, error) {
		launched = path
		return chrome.CDPProbeResult{Port: 9222}, nil
	}
	t.Cleanup(func() { launchAndProbe = original })

	result := cdpReachableCheck(context.Background(), &browsers.DoctorEnv{})
	if result.Status != browsers.DoctorPass {
		t.Fatalf("status = %v, want pass: %s", result.Status, result.Detail)
	}
	if launched != binary {
		t.Fatalf("launched %q, want discovered binary %q", launched, binary)
	}
}

func TestFingerprintFlagsAllowColdStart(t *testing.T) {
	original := launchAndProbe
	launchAndProbe = func(_ context.Context, _ string, _ []string, timeout time.Duration) (chrome.CDPProbeResult, error) {
		if timeout < 22*time.Second {
			return chrome.CDPProbeResult{}, context.DeadlineExceeded
		}
		return chrome.CDPProbeResult{Port: 9222}, nil
	}
	t.Cleanup(func() { launchAndProbe = original })

	env := &browsers.DoctorEnv{
		Binary: "/cloakbrowser",
		Cloak:  browsers.CloakFingerprint{FingerprintSeed: "test-seed"},
	}
	result := fingerprintFlagsCheck(context.Background(), env)
	if result.Status != browsers.DoctorPass {
		t.Fatalf("status = %v, want pass after cold start: %s", result.Status, result.Detail)
	}
}
