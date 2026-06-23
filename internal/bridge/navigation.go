package bridge

import (
	"context"

	"github.com/chromedp/chromedp"
)

func (b *Bridge) CurrentURL(ctx context.Context) (string, error) {
	var url string
	err := chromedp.Run(ctx, chromedp.Location(&url))
	return url, err
}

func (b *Bridge) CurrentTitle(ctx context.Context) (string, error) {
	var title string
	err := chromedp.Run(ctx, chromedp.Title(&title))
	return title, err
}
