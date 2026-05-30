package bridge

import (
	"context"

	"github.com/chromedp/chromedp"
)

// CurrentURL returns the current page URL.
func (b *Bridge) CurrentURL(ctx context.Context) (string, error) {
	var url string
	err := chromedp.Run(ctx, chromedp.Location(&url))
	return url, err
}

// CurrentTitle returns the current page title.
func (b *Bridge) CurrentTitle(ctx context.Context) (string, error) {
	var title string
	err := chromedp.Run(ctx, chromedp.Title(&title))
	return title, err
}
