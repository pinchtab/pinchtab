package config

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
)

const (
	BrowserChrome      = "chrome"
	BrowserCloak       = "cloak"
	BrowserGhostChrome = "ghost-chrome"
)

// NormalizeBrowser maps user-facing config values (browser.provider,
// CLI flags) to canonical browser registry IDs. It is the intentional gateway
// between the config/API layer and browsers.Get during the browser architecture
// migration.
func NormalizeBrowser(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case BrowserCloak:
		return BrowserCloak
	case BrowserGhostChrome:
		return BrowserGhostChrome
	default:
		return BrowserChrome
	}
}

func ParseBrowser(name string, configured []string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return BrowserChrome, nil
	}
	for _, c := range configured {
		if strings.ToLower(strings.TrimSpace(c)) == normalized {
			return normalized, nil
		}
	}
	if _, ok := browsers.Get(normalized); ok {
		return normalized, nil
	}
	if len(configured) > 0 {
		return "", fmt.Errorf("unknown browser %q; configured: %v, built-in: %v", name, configured, browsers.IDs())
	}
	return "", fmt.Errorf("unknown browser %q (known: %v)", name, browsers.IDs())
}

// ResolveBrowser applies the browser precedence chain:
// request > session > instance > global default > configuredOrder[0] > chrome.
// Each level is checked in order; the first non-empty value wins.
func ResolveBrowser(request, session, instance, globalDefault string, configuredOrder []string) string {
	if request != "" {
		return request
	}
	if session != "" {
		return session
	}
	if instance != "" {
		return instance
	}
	if globalDefault != "" {
		return globalDefault
	}
	if len(configuredOrder) > 0 {
		return configuredOrder[0]
	}
	return BrowserChrome
}

// IsCloakBrowser gates Cloak-specific fingerprint behavior in
// stealth and runtime paths.
func IsCloakBrowser(provider string) bool {
	return NormalizeBrowser(provider) == BrowserCloak
}

// CloakBrowserActive reports whether the configured browser is Cloak. It
// deliberately does not mean PinchTab's stealth layer should be disabled.
func CloakBrowserActive(cfg *RuntimeConfig) bool {
	if cfg == nil || !IsCloakBrowser(cfg.DefaultBrowser) {
		return false
	}
	return true
}

// PinchTabStealthDefaultsDisabled is the explicit opt-out from PinchTab's JS
// stealth overlays and automation-hiding launch flags.
func PinchTabStealthDefaultsDisabled(cfg *RuntimeConfig) bool {
	if !CloakBrowserActive(cfg) {
		return false
	}
	return cfg.Cloak.DisableDefaultStealthArgs
}

func hasCloakBrowserConfig(c CloakBrowserConfig) bool {
	return c.FingerprintSeed != "" ||
		c.Platform != "" ||
		c.Locale != "" ||
		c.Timezone != "" ||
		c.WebRTCIP != "" ||
		c.FontsDir != "" ||
		c.StorageQuotaMB != nil ||
		c.DisableDefaultStealthArgs != nil
}
