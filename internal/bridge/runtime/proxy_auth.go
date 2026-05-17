package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

// proxyAuthEnabled is the single gate for the CDP credential path
// (testable without a running Chrome).
func proxyAuthEnabled(p config.BrowserProxyConfig) bool {
	if p.IsZero() {
		return false
	}
	return p.Username != ""
}

// enableProxyAuth registers a browser-scoped CDP Fetch listener that responds
// to proxy auth challenges. Never log the password — use p.Redacted().
func enableProxyAuth(browserCtx context.Context, p config.BrowserProxyConfig) error {
	if !proxyAuthEnabled(p) {
		return nil
	}

	if err := chromedp.Run(browserCtx,
		fetch.Enable().WithHandleAuthRequests(true),
	); err != nil {
		return fmt.Errorf("enable fetch domain for proxy auth: %w", err)
	}

	username := p.Username
	password := p.Password

	chromedp.ListenBrowser(browserCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			handleAuthRequired(browserCtx, e, username, password)
		case *fetch.EventRequestPaused:
			handleRequestPaused(browserCtx, e)
		}
	})

	slog.Info("proxy authentication enabled via CDP", "proxy", p.Redacted())
	return nil
}

func handleAuthRequired(browserCtx context.Context, e *fetch.EventAuthRequired, username, password string) {
	// Yield non-proxy challenges (e.g. server WWW-Authenticate) with Default.
	if e.AuthChallenge == nil || e.AuthChallenge.Source != fetch.AuthChallengeSourceProxy {
		if err := chromedp.Run(browserCtx,
			fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
				Response: fetch.AuthChallengeResponseResponseDefault,
			}),
		); err != nil && browserCtx.Err() == nil {
			slog.Warn("proxy auth: yield (non-proxy) failed", "err", err)
		}
		return
	}
	if err := chromedp.Run(browserCtx,
		fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
			Response: fetch.AuthChallengeResponseResponseProvideCredentials,
			Username: username,
			Password: password,
		}),
	); err != nil && browserCtx.Err() == nil {
		slog.Warn("proxy auth: provide credentials failed", "err", err)
	}
}

func handleRequestPaused(browserCtx context.Context, e *fetch.EventRequestPaused) {
	if err := chromedp.Run(browserCtx, fetch.ContinueRequest(e.RequestID)); err != nil && browserCtx.Err() == nil {
		slog.Warn("proxy auth: continue request failed", "err", err)
	}
}
