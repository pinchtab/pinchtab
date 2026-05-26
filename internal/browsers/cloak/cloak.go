// Package cloak registers a CloakBrowser provider.
// It embeds chrome.Browser and overrides only identity and capabilities.
package cloak

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

// binaryNames lists the executable names that may represent a CloakBrowser
// install on any platform.
var binaryNames = []string{
	"cloakbrowser",
}

// BinaryNames returns the set of CloakBrowser binary names used during
// discovery. The returned slice is a defensive copy.
func BinaryNames() []string {
	out := make([]string, len(binaryNames))
	copy(out, binaryNames)
	return out
}

// CommonPaths returns per-OS CloakBrowser install paths probed after PATH
// misses.
func CommonPaths(goos string) []string {
	switch goos {
	case "linux", "darwin":
		paths := []string{
			"/opt/cloakbrowser/chrome",
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			paths = append(paths, filepath.Join(home, ".cloakbrowser", "chrome"))
		}
		return paths
	default:
		return nil
	}
}

// Browser implements browsers.Browser for CloakBrowser.
// It embeds chrome.Browser and overrides only the methods that differ.
type Browser struct{ chrome.Browser }

func (Browser) ID() string          { return "cloak" }
func (Browser) DisplayName() string { return "CloakBrowser" }

func (Browser) Capabilities() browsers.CapabilitySet {
	return browsers.NewCapabilitySet(
		browsers.CapCDP,
		browsers.CapHeadless,
		browsers.CapPDF,
		browsers.CapExtensions,
		browsers.CapDownloads,
		browsers.CapNetworkInterception,
		browsers.CapNativeStealth, // Cloak-only
	)
}

// BuildLaunchArgs extends the Chrome base flags with CloakBrowser-specific
// fingerprint flags drawn from cfg.Cloak.
func (b Browser) BuildLaunchArgs(cfg browsers.LaunchConfig) ([]string, []string, error) {
	cfg.Mode = browsers.ResolveLaunchMode(cfg.Mode)
	if cfg.Mode == browsers.LaunchModeLite {
		return nil, nil, fmt.Errorf("cloak provider does not support %q launch mode", cfg.Mode)
	}
	args, env, err := b.Browser.BuildLaunchArgs(cfg)
	if err != nil {
		return nil, nil, err
	}

	c := cfg.Cloak
	addFlag := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			args = append(args, name+"="+value)
		}
	}
	addFlag("--fingerprint", c.FingerprintSeed)
	addFlag("--fingerprint-platform", c.Platform)
	addFlag("--fingerprint-locale", c.Locale)
	addFlag("--fingerprint-timezone", c.Timezone)
	addFlag("--fingerprint-webrtc-ip", c.WebRTCIP)
	addFlag("--fingerprint-fonts-dir", c.FontsDir)
	if c.StorageQuotaMB > 0 {
		args = append(args, "--fingerprint-storage-quota="+strconv.Itoa(c.StorageQuotaMB))
	}

	return args, env, nil
}

// DiscoverBinary locates a CloakBrowser binary on the current system,
// overriding the inherited Chrome discovery.
func (Browser) DiscoverBinary() browsers.BinaryDiscovery {
	d := browserprobe.DiscoverBinary(BinaryNames(), CommonPaths(runtime.GOOS))
	return browsers.BinaryDiscovery{Found: d.Found, Probed: d.Probed}
}

func (Browser) ValidateTarget(cfg browsers.TargetConfig) error {
	if strings.TrimSpace(cfg.Binary) == "" {
		return fmt.Errorf("must be set when provider is cloak")
	}
	return nil
}

func (Browser) ClassifyLaunchError(f browsers.LaunchFailure) browsers.LaunchErrorKind {
	if f.Err == nil {
		return browsers.LaunchErrorUnknown
	}
	if f.ParentCanceled {
		return browsers.LaunchErrorUnknown
	}
	if !f.BrowserCanceled {
		return browsers.LaunchErrorUnknown
	}
	isCanceled := errors.Is(f.Err, context.Canceled) || strings.Contains(f.Err.Error(), "context canceled")
	if !isCanceled {
		return browsers.LaunchErrorUnknown
	}
	if f.Elapsed >= 5*time.Second {
		return browsers.LaunchErrorUnknown
	}
	return browsers.LaunchErrorSilentCDPDrop
}

func (Browser) GeoAlignment(geo browsers.GeoConfig) browsers.GeoStrategy {
	var flags []string
	if geo.Timezone != "" && geo.OperatorTimezone == "" {
		flags = append(flags, "--fingerprint-timezone="+geo.Timezone)
	}
	if geo.Locale != "" && geo.OperatorLocale == "" {
		flags = append(flags, "--fingerprint-locale="+geo.Locale)
	}
	if geo.WebRTCIP != "" && geo.OperatorWebRTCIP == "" {
		flags = append(flags, "--fingerprint-webrtc-ip="+geo.WebRTCIP)
	}
	return browsers.GeoStrategy{
		Flags:        flags,
		OperatorWins: true,
	}
}

func init() { browsers.Register(&Browser{}) }
