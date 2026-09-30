package bridge

import (
	"github.com/chromedp/cdproto/emulation"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func buildUserAgentOverride(userAgent, chromeVersion string) *emulation.SetUserAgentOverrideParams {
	return stealth.BuildUserAgentOverride(userAgent, chromeVersion)
}

// userAgentOverrideAction builds the CDP override for a caller-supplied identity: the
// persona's client-hint metadata for that UA, with the caller's own platform and
// accept-language laid over it. The metadata is the point — Chrome stops emitting every
// Sec-CH-UA header once setUserAgentOverride arrives without it, so a rotated tab
// advertised a UA that no client hint backed, which is a louder signal than the identity
// it was meant to present.
func userAgentOverrideAction(params UserAgentOverrideParams, chromeVersion string) *emulation.SetUserAgentOverrideParams {
	override := buildUserAgentOverride(params.UserAgent, chromeVersion)
	if override == nil {
		override = emulation.SetUserAgentOverride(params.UserAgent)
	}
	if params.Platform != "" {
		override = override.WithPlatform(params.Platform)
	}
	if params.AcceptLanguage != "" {
		override = override.WithAcceptLanguage(params.AcceptLanguage)
	}
	return override
}
