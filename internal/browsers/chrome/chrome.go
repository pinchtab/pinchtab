// Package chrome registers a Google Chrome browser provider.
package chrome

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/browsers"
)

// binaryNames lists the executable names that may represent a Chrome or
// Chromium install on any platform.
var binaryNames = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
	"chrome",
}

// BinaryNames returns the set of Chrome/Chromium binary names used during
// discovery. The returned slice is a defensive copy.
func BinaryNames() []string {
	out := make([]string, len(binaryNames))
	copy(out, binaryNames)
	return out
}

// CommonPaths returns per-OS Chrome install paths probed after PATH misses.
func CommonPaths(goos string) []string {
	switch goos {
	case "linux":
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/opt/google/chrome/chrome",
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		}
	case "windows":
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	default:
		return nil
	}
}

func existingExtensionPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	valid := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			valid = append(valid, p)
		}
	}
	return valid
}

func randomWindowSize() (int, int) {
	sizes := [][2]int{
		{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
		{1280, 720}, {1600, 900}, {2560, 1440}, {1280, 800},
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(sizes))))
	idx := 0
	if err == nil {
		idx = int(n.Int64())
	}
	s := sizes[idx]
	return s[0], s[1]
}

// Browser implements browsers.Browser for Google Chrome.
type Browser struct{}

func (Browser) ID() string          { return "chrome" }
func (Browser) DisplayName() string { return "Google Chrome" }

func (Browser) Capabilities() browsers.CapabilitySet {
	return browsers.NewCapabilitySet(
		browsers.CapCDP,
		browsers.CapHeadless,
		browsers.CapPDF,
		browsers.CapExtensions,
		browsers.CapDownloads,
		browsers.CapNetworkInterception,
		browsers.CapEventScreencast,
	)
}

// DiscoverBinary locates a Chrome/Chromium binary on the current system.
func (Browser) DiscoverBinary() browsers.BinaryDiscovery {
	d := browserprobe.DiscoverBinary(BinaryNames(), CommonPaths(runtime.GOOS))
	return browsers.BinaryDiscovery{Found: d.Found, Probed: d.Probed}
}

func (Browser) BuildLaunchArgs(cfg browsers.LaunchConfig) ([]string, []string, error) {
	cfg.Mode = browsers.ResolveLaunchMode(cfg.Mode)
	if cfg.Mode == browsers.LaunchModeLite {
		return nil, nil, fmt.Errorf("chrome provider does not support %q launch mode", cfg.Mode)
	}
	var args []string

	// Debug port (first, before base flags)
	if cfg.DebugPort > 0 {
		args = append(args, fmt.Sprintf("--remote-debugging-port=%d", cfg.DebugPort))
	}

	args = append(args,
		"--disable-background-networking",
		"--enable-features=NetworkService,NetworkServiceInProcess",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-breakpad",
		"--disable-session-crashed-bubble",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--disable-features=Translate,BlinkGenPropertyTrees",
		"--hide-crash-restore-bubble",
		"--disable-hang-monitor",
		"--disable-ipc-flooding-protection",
		"--disable-metrics-reporting",
		"--disable-prompt-on-repost",
		"--disable-renderer-backgrounding",
		"--disable-sync",
		"--force-color-profile=srgb",
		"--metrics-recording-only",
		"--noerrdialogs",
		"--safebrowsing-disable-auto-update",
		"--password-store=basic",
		"--use-mock-keychain",
	)

	if cfg.Headless {
		// No --disable-gpu here: under --headless=new the compositor needs a
		// GPU backend (swiftshader, enabled below); disabling the GPU process
		// leaves Page.captureScreenshot/printToPDF with no backend and they
		// hang past the action timeout.
		args = append(args,
			"--headless=new",
			"--disable-vulkan",
			"--use-angle=swiftshader",
			"--enable-unsafe-swiftshader",
		)
	}

	if validPaths := existingExtensionPaths(cfg.ExtensionPaths); len(validPaths) > 0 {
		joined := strings.Join(validPaths, ",")
		args = append(args, "--load-extension="+joined, "--disable-extensions-except="+joined)
	} else {
		args = append(args, "--disable-extensions")
	}

	if cfg.ProfileDir != "" {
		args = append(args, "--user-data-dir="+cfg.ProfileDir)
	}

	w, h := randomWindowSize()
	args = append(args, fmt.Sprintf("--window-size=%d,%d", w, h))

	if cfg.Timezone != "" {
		args = append(args, "--tz="+cfg.Timezone)
	}

	// Extra flags (caller pre-filters)
	args = append(args, cfg.ExtraFlags...)

	if cfg.NoSandbox {
		args = append(args, "--no-sandbox")
	}

	return args, nil, nil
}

func (Browser) SupportsRemoteCDP() bool                                { return true }
func (Browser) GeoAlignment(_ browsers.GeoConfig) browsers.GeoStrategy { return browsers.GeoStrategy{} }
func (Browser) ValidateTarget(_ browsers.TargetConfig) error           { return nil }

func (Browser) ClassifyLaunchError(_ browsers.LaunchFailure) browsers.LaunchErrorKind {
	return browsers.LaunchErrorUnknown
}

func (Browser) CanHandle(_ browsers.RequestIntent) browsers.HandleDecision {
	return browsers.HandleDecision{Decision: browsers.DecisionHandle}
}

func (Browser) NewRuntimeInstance(browserCtx context.Context, headless bool) browsers.RuntimeInstance {
	return NewInstance(browserCtx, headless)
}

func init() { browsers.Register(&Browser{}) }
