// Package uameta builds CDP UserAgentMetadata from config.
package uameta

import (
	"runtime"
	"strings"

	"github.com/chromedp/cdproto/emulation"
)

// Build creates a SetUserAgentOverride action with full UserAgentMetadata.
// chromeVersion should be the full version (e.g. "144.0.7559.133").
// If userAgent is empty, generates a default one based on the platform.
// If chromeVersion is also empty, returns nil.
func Build(userAgent, chromeVersion string) *emulation.SetUserAgentOverrideParams {
	if chromeVersion == "" {
		return nil
	}

	// If no custom user agent is provided, generate a default one based on OS
	if userAgent == "" {
		switch runtime.GOOS {
		case "darwin":
			userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
		case "windows":
			userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
		default:
			userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
		}
	}

	major := chromeVersion
	if i := strings.Index(chromeVersion, "."); i > 0 {
		major = chromeVersion[:i]
	}

	platform, arch := detectPlatform()

	return emulation.SetUserAgentOverride(userAgent).
		WithAcceptLanguage("en-US,en").
		WithPlatform(platform).
		WithUserAgentMetadata(&emulation.UserAgentMetadata{
			Platform:        platformName(),
			PlatformVersion: platformVersion(),
			Architecture:    arch,
			Bitness:         "64",
			Mobile:          false,
			Brands: []*emulation.UserAgentBrandVersion{
				{Brand: "Not(A:Brand", Version: "99"},
				{Brand: "Google Chrome", Version: major},
				{Brand: "Chromium", Version: major},
			},
			FullVersionList: []*emulation.UserAgentBrandVersion{
				{Brand: "Not(A:Brand", Version: "99.0.0.0"},
				{Brand: "Google Chrome", Version: chromeVersion},
				{Brand: "Chromium", Version: chromeVersion},
			},
		})
}

func detectPlatform() (jsNavigatorPlatform, architecture string) {
	switch runtime.GOARCH {
	case "arm64":
		architecture = "arm"
	default:
		architecture = "x86"
	}

	switch runtime.GOOS {
	case "darwin":
		return "MacIntel", architecture
	case "windows":
		return "Win32", architecture
	default:
		return "Linux x86_64", architecture
	}
}

func platformName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	default:
		return "Linux"
	}
}

func platformVersion() string {
	switch runtime.GOOS {
	case "darwin":
		return "14.0.0"
	case "windows":
		return "15.0.0"
	default:
		return "6.5.0"
	}
}
