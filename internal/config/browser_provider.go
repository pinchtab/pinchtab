package config

import (
	"fmt"
	"strings"
)

const (
	BrowserProviderChrome = "chrome"
	BrowserProviderCloak  = "cloak"
)

func NormalizeBrowserProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case BrowserProviderCloak:
		return BrowserProviderCloak
	default:
		return BrowserProviderChrome
	}
}

func ParseBrowserProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", BrowserProviderChrome:
		return BrowserProviderChrome, nil
	case BrowserProviderCloak:
		return BrowserProviderCloak, nil
	default:
		return "", fmt.Errorf("invalid browser provider %q (must be chrome or cloak)", provider)
	}
}

func IsCloakBrowserProvider(provider string) bool {
	return NormalizeBrowserProvider(provider) == BrowserProviderCloak
}

// CloakBrowserProviderActive reports only the configured browser provider. It
// deliberately does not mean PinchTab's stealth layer should be disabled.
func CloakBrowserProviderActive(cfg *RuntimeConfig) bool {
	if cfg == nil || !IsCloakBrowserProvider(cfg.BrowserProvider) {
		return false
	}
	return true
}

// PinchTabStealthDefaultsDisabled is the explicit opt-out from PinchTab's JS
// stealth overlays and automation-hiding launch flags.
func PinchTabStealthDefaultsDisabled(cfg *RuntimeConfig) bool {
	if !CloakBrowserProviderActive(cfg) {
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
