package config

import "strings"

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

func IsCloakBrowserProvider(provider string) bool {
	return NormalizeBrowserProvider(provider) == BrowserProviderCloak
}

func NativeCloakStealthEnabled(cfg *RuntimeConfig) bool {
	return cfg != nil &&
		IsCloakBrowserProvider(cfg.BrowserProvider) &&
		cfg.Cloak.DisableDefaultStealthArgs
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
