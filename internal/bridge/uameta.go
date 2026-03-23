package bridge

import (
	"github.com/chromedp/cdproto/emulation"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func buildUserAgentOverride(userAgent, chromeVersion string) *emulation.SetUserAgentOverrideParams {
	return stealth.BuildUserAgentOverride(userAgent, chromeVersion)
}

func buildLocaleOverride(userAgent, chromeVersion string) *emulation.SetLocaleOverrideParams {
	return stealth.BuildLocaleOverride(userAgent, chromeVersion)
}
