package config

import "strings"

// BrowserCapability is a well-known capability name that a provider may advertise. Advisory only.
type BrowserCapability string

const (
	CapCDP                 BrowserCapability = "cdp"
	CapHeadless            BrowserCapability = "headless"
	CapPDF                 BrowserCapability = "pdf"
	CapExtensions          BrowserCapability = "extensions"
	CapNativeStealth       BrowserCapability = "nativeStealth"
	CapDownloads           BrowserCapability = "downloads"
	CapNetworkInterception BrowserCapability = "networkInterception"
)

// providerCapabilities is declared in stable order so JSON serialization is deterministic.
var providerCapabilities = map[string][]BrowserCapability{
	BrowserProviderChrome: {
		CapCDP,
		CapHeadless,
		CapPDF,
		CapExtensions,
		CapDownloads,
		CapNetworkInterception,
	},
	BrowserProviderCloak: {
		CapCDP,
		CapHeadless,
		CapPDF,
		CapExtensions,
		CapDownloads,
		CapNetworkInterception,
		CapNativeStealth,
	},
}

// lookupProviderCapabilities returns ok=false for unknown providers so callers can distinguish "no
// caps declared" from NormalizeBrowserProvider's default-to-chrome fallback.
func lookupProviderCapabilities(provider string) ([]BrowserCapability, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(provider))
	if _, known := providerCapabilities[trimmed]; !known {
		return nil, false
	}
	normalized := NormalizeBrowserProvider(provider)
	src, ok := providerCapabilities[normalized]
	return src, ok
}

// ProviderCapabilities returns a defensive copy; unknown providers yield an empty slice.
func ProviderCapabilities(provider string) []BrowserCapability {
	src, ok := lookupProviderCapabilities(provider)
	if !ok {
		return nil
	}
	out := make([]BrowserCapability, len(src))
	copy(out, src)
	return out
}

// HasCapability reports whether provider declares c; unknown providers return false.
func HasCapability(provider string, c BrowserCapability) bool {
	src, ok := lookupProviderCapabilities(provider)
	if !ok {
		return false
	}
	for _, cap := range src {
		if cap == c {
			return true
		}
	}
	return false
}
