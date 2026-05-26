package handlers

import (
	"context"

	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/routing"
)

// routeRequest evaluates browser routing via the provider router for any
// browser. When ghost-chrome routing succeeds with "ghost", the GhostResult
// in the returned Result contains the already-fetched content. For chrome/cloak
// the result carries routing metadata but no GhostResult.
func (h *Handlers) routeRequest(ctx context.Context, url, browser string, intent browsers.RequestIntent) *routing.Result {
	var fetcher ghostchrome.StaticFetcher
	if h.StaticBrowser != nil {
		fetcher = &staticFetcher{browser: h.StaticBrowser}
	}
	// LiteFetcher can be nil for non-ghost-chrome browsers — they don't use it.
	result, err := routing.Route(routing.Request{
		Ctx:         ctx,
		URL:         url,
		Intent:      intent,
		Browser:     browser,
		LiteFetcher: fetcher,
	})
	if err != nil {
		return nil // routing failed, fall through to chrome
	}
	return &result
}

// staticFetcher adapts browserops.BrowserRuntime to ghostchrome.StaticFetcher.
type staticFetcher struct {
	browser browserops.BrowserRuntime
}

func (f *staticFetcher) Navigate(ctx context.Context, url string) (ghostchrome.StaticNavResult, error) {
	r, err := f.browser.Navigate(ctx, url)
	if err != nil {
		return ghostchrome.StaticNavResult{}, err
	}
	return ghostchrome.StaticNavResult{
		TabID: r.TabID,
		URL:   r.URL,
		Title: r.Title,
	}, nil
}

func (f *staticFetcher) Text(ctx context.Context, tabID string) (ghostchrome.StaticTextResult, error) {
	r, err := f.browser.Text(ctx, tabID)
	if err != nil {
		return ghostchrome.StaticTextResult{}, err
	}
	return ghostchrome.StaticTextResult{
		Text: r.Text,
	}, nil
}
