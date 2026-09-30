package bridge

import (
	"context"

	"github.com/chromedp/cdproto/emulation"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

func ResolveUnifiedSelector(ctx context.Context, sel selector.Selector, refCache *RefCache) (int64, error) {
	return ResolveUnifiedSelectorInFrame(ctx, sel, refCache, "")
}

func buildLocaleOverride(userAgent, chromeVersion string) *emulation.SetLocaleOverrideParams {
	return stealth.BuildLocaleOverride(userAgent, chromeVersion)
}
