package runtime

// Provider-depth contract:
//   - Cloak: deep alignment via --fingerprint-* flags; explicit per-target
//     Cloak fingerprint config wins over derived geo (operator intent beats
//     proxy-driven hints).
//   - Chrome: no automatic proxy-derived geo localization. Use Cloak when
//     PinchTab should align browser fingerprint fields with proxy geography.
//   - Unknown providers: no-op.

import (
	"github.com/pinchtab/pinchtab/internal/browsers"
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
	browserID := config.NormalizeBrowser(provider)
	b, ok := browsers.Get(browserID)
	if !ok {
		return nil, nil
	}
	gc := browsers.GeoConfig{
		Timezone:         info.Timezone,
		Locale:           info.Locale,
		WebRTCIP:         info.WebRTCIP,
		CountryISO:       info.CountryISO,
		OperatorTimezone: cloak.Timezone,
		OperatorLocale:   cloak.Locale,
		OperatorWebRTCIP: cloak.WebRTCIP,
	}
	gs := b.GeoAlignment(gc)
	return gs.Flags, gs.Env
}
