package runtime

// Provider-depth contract:
//   - Cloak: deep alignment via --fingerprint-* flags; explicit per-target
//     Cloak fingerprint config wins over derived geo (operator intent beats
//     proxy-driven hints).
//   - Chrome: no automatic proxy-derived geo localization. Use Cloak when
//     PinchTab should align browser fingerprint fields with proxy geography.
//   - Unknown providers: no-op.

import (
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/geo"
)

// applyGeoAlignment returns provider-specific flags and env additions.
// cloak carries the merged Cloak runtime config so the "explicit wins"
// precedence rule is honored without re-reading config.
func applyGeoAlignment(provider string, info geo.Info, cloak config.CloakBrowserRuntimeConfig) (flags, env []string) {
	if info.IsZero() {
		return nil, nil
	}
	switch config.NormalizeBrowserProvider(provider) {
	case config.BrowserProviderCloak:
		return cloakGeoFlags(info, cloak), nil
	default:
		return nil, nil
	}
}

// Skips fields the operator has explicitly set on the Cloak config —
// per-target intent (e.g. NY locale via London proxy) wins.
func cloakGeoFlags(info geo.Info, cloak config.CloakBrowserRuntimeConfig) []string {
	var flags []string
	if info.Timezone != "" && cloak.Timezone == "" {
		flags = append(flags, "--fingerprint-timezone="+info.Timezone)
	}
	if info.Locale != "" && cloak.Locale == "" {
		flags = append(flags, "--fingerprint-locale="+info.Locale)
	}
	if info.WebRTCIP != "" && cloak.WebRTCIP == "" {
		flags = append(flags, "--fingerprint-webrtc-ip="+info.WebRTCIP)
	}
	return flags
}
