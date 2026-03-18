package cdpops

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const TargetTypePage = "page"

func NavigatePage(ctx context.Context, url string) error {
	replaceInitialBlank, _ := shouldReplaceInitialBlankNavigation(ctx)
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return startNavigation(ctx, url, replaceInitialBlank)
	}))
	if err != nil {
		return err
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var state string
			err = chromedp.Run(ctx, chromedp.Evaluate("document.readyState", &state))
			if err == nil && (state == "interactive" || state == "complete") {
				return nil
			}
		}
	}
}

var ErrTooManyRedirects = fmt.Errorf("too many redirects")

func NavigatePageWithRedirectLimit(ctx context.Context, url string, maxRedirects int) error {
	replaceInitialBlank, _ := shouldReplaceInitialBlankNavigation(ctx)

	if maxRedirects < 0 {
		return navigateAndWait(ctx, url, replaceInitialBlank)
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().Do(ctx)
	})); err != nil {
		return fmt.Errorf("fetch enable: %w", err)
	}

	var redirectCount atomic.Int32
	var blocked atomic.Bool

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		go func() {
			reqID := e.RequestID
			if e.RedirectedRequestID != "" {
				count := int(redirectCount.Add(1))
				if count > maxRedirects {
					blocked.Store(true)
					_ = fetch.FailRequest(reqID, network.ErrorReasonBlockedByClient).Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Target))
					return
				}
			}
			_ = fetch.ContinueRequest(reqID).Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Target))
		}()
	})

	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return startNavigation(ctx, url, replaceInitialBlank)
	}))

	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Disable().Do(ctx)
	}))

	if blocked.Load() {
		return fmt.Errorf("%w: got %d, max %d", ErrTooManyRedirects, redirectCount.Load(), maxRedirects)
	}
	if err != nil {
		return err
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var state string
			err := chromedp.Run(ctx, chromedp.Evaluate("document.readyState", &state))
			if err == nil && (state == "interactive" || state == "complete") {
				return nil
			}
		}
	}
}

// ShouldReplaceBlankHistoryEntry reports whether the first navigation should replace an untouched about:blank entry.
func ShouldReplaceBlankHistoryEntry(curURL string, cur int64, entryCount int) bool {
	return curURL == "about:blank" && cur == 0 && entryCount == 1
}

func shouldReplaceInitialBlankNavigation(ctx context.Context) (bool, error) {
	var (
		cur     int64
		entries []*page.NavigationEntry
		curURL  string
	)

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cur, entries, err = page.GetNavigationHistory().Do(ctx)
			return err
		}),
		chromedp.Location(&curURL),
	); err != nil {
		return false, err
	}

	return ShouldReplaceBlankHistoryEntry(curURL, cur, len(entries)), nil
}

func startNavigation(ctx context.Context, url string, replaceInitialBlank bool) error {
	if !replaceInitialBlank {
		_, _, _, _, err := page.Navigate(url).Do(ctx)
		return err
	}

	encodedURL, err := json.Marshal(url)
	if err != nil {
		return fmt.Errorf("encode navigation url: %w", err)
	}

	return chromedp.Evaluate("window.location.replace("+string(encodedURL)+")", nil).Do(ctx)
}

func navigateAndWait(ctx context.Context, url string, replaceInitialBlank bool) error {
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return startNavigation(ctx, url, replaceInitialBlank)
	})); err != nil {
		return err
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var state string
			err := chromedp.Run(ctx, chromedp.Evaluate("document.readyState", &state))
			if err == nil && (state == "interactive" || state == "complete") {
				return nil
			}
		}
	}
}

func WaitForTitle(ctx context.Context, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		var title string
		if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
			return "", err
		}
		return title, nil
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				return "", err
			}
			return title, nil
		case <-ticker.C:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				continue
			}
			if title != "" && title != "about:blank" {
				return title, nil
			}
		}
	}
}
