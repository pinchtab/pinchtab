package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func TestChromeNeedsNoSandbox(t *testing.T) {
	origGOOS := runtimeGOOS
	origGeteuid := osGeteuid
	origMarker := containerMarkerPath
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osGeteuid = origGeteuid
		containerMarkerPath = origMarker
	})

	runtimeGOOS = "linux"
	osGeteuid = func() int { return 1000 }
	containerMarkerPath = t.TempDir() + "/missing-dockerenv"

	if chromeNeedsNoSandbox() {
		t.Fatal("expected no-sandbox compatibility to be disabled without root or container marker")
	}

	osGeteuid = func() int { return 0 }
	if !chromeNeedsNoSandbox() {
		t.Fatal("expected root user to enable no-sandbox compatibility")
	}
	osGeteuid = func() int { return 1000 }

	containerMarkerPath = t.TempDir() + "/.dockerenv"
	if err := os.WriteFile(containerMarkerPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if !chromeNeedsNoSandbox() {
		t.Fatal("expected container marker to enable no-sandbox compatibility")
	}
}

func TestShouldRetryChromeStartupWithDirectLaunch(t *testing.T) {
	canceledParent, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name      string
		parentCtx context.Context
		err       error
		want      bool
	}{
		{
			name:      "startup timeout retries",
			parentCtx: context.Background(),
			err:       context.DeadlineExceeded,
			want:      true,
		},
		{
			name:      "allocator context canceled retries",
			parentCtx: context.Background(),
			err:       context.Canceled,
			want:      true,
		},
		{
			name:      "wrapped context canceled retries",
			parentCtx: context.Background(),
			err:       fmt.Errorf("failed to start: %w", context.Canceled),
			want:      true,
		},
		{
			name:      "string matched context canceled retries",
			parentCtx: context.Background(),
			err:       errors.New("failed to connect to chrome: context canceled"),
			want:      true,
		},
		{
			name:      "parent cancellation does not retry",
			parentCtx: canceledParent,
			err:       context.Canceled,
			want:      false,
		},
		{
			name:      "other errors do not retry",
			parentCtx: context.Background(),
			err:       errors.New("exec format error"),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryChromeStartupWithDirectLaunch(tt.parentCtx, tt.err); got != tt.want {
				t.Fatalf("shouldRetryChromeStartupWithDirectLaunch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildChromeArgs_CloakProviderUsesNativeFingerprintFlags(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderCloak,
		Cloak: config.CloakBrowserRuntimeConfig{
			FingerprintSeed:           "42069",
			Platform:                  "windows",
			Locale:                    "en-GB",
			Timezone:                  "Europe/London",
			WebRTCIP:                  "auto",
			FontsDir:                  "/opt/fonts",
			StorageQuotaMB:            2048,
			DisableDefaultStealthArgs: true,
		},
	}

	args := BuildChromeArgs(cfg, 9222)
	for _, want := range []string{
		"--fingerprint=42069",
		"--fingerprint-platform=windows",
		"--fingerprint-locale=en-GB",
		"--fingerprint-timezone=Europe/London",
		"--fingerprint-webrtc-ip=auto",
		"--fingerprint-fonts-dir=/opt/fonts",
		"--fingerprint-storage-quota=2048",
	} {
		if !stealth.HasLaunchArg(args, want) {
			t.Fatalf("BuildChromeArgs() missing %q in %v", want, args)
		}
	}
	for _, blocked := range []string{
		"--disable-automation",
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
	} {
		if stealth.HasLaunchArg(args, blocked) {
			t.Fatalf("BuildChromeArgs() included PinchTab stealth arg %q in native Cloak mode: %v", blocked, args)
		}
	}
	if stealth.HasLaunchArgPrefix(args, "--user-agent=") {
		t.Fatalf("BuildChromeArgs() included PinchTab user-agent override in native Cloak mode: %v", args)
	}
}

func TestBuildChromeArgs_DefaultChromeProviderKeepsChromeLaunchContract(t *testing.T) {
	cfg := &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderChrome,
		ChromeVersion:   "144.0.0.0",
		ExtensionPaths:  []string{},
	}

	args := BuildChromeArgs(cfg, 9222)
	for _, want := range []string{
		"--remote-debugging-port=9222",
		"--disable-background-networking",
		"--disable-automation",
		"--enable-automation=false",
		"--disable-blink-features=AutomationControlled",
		"--enable-network-information-downlink-max",
		"--lang=en-US",
		"--disable-extensions",
	} {
		if !stealth.HasLaunchArg(args, want) {
			t.Fatalf("BuildChromeArgs() missing Chrome provider arg %q in %v", want, args)
		}
	}
	if !stealth.HasLaunchArgPrefix(args, "--user-agent=Mozilla/5.0") {
		t.Fatalf("BuildChromeArgs() missing Chrome provider user-agent in %v", args)
	}
	for _, blockedPrefix := range []string{
		"--fingerprint=",
		"--fingerprint-platform=",
		"--fingerprint-locale=",
		"--fingerprint-timezone=",
		"--fingerprint-webrtc-ip=",
		"--fingerprint-fonts-dir=",
		"--fingerprint-storage-quota=",
	} {
		if stealth.HasLaunchArgPrefix(args, blockedPrefix) {
			t.Fatalf("BuildChromeArgs() included Cloak flag prefix %q in Chrome provider mode: %v", blockedPrefix, args)
		}
	}
}
