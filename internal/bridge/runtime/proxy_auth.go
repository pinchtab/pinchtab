package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

// ProxyAuthEnabled is the single gate for the CDP credential path
// (testable without a running Chrome).
func ProxyAuthEnabled(p config.BrowserProxyConfig) bool {
	if p.IsZero() {
		return false
	}
	return p.Username != ""
}

// EnableProxyAuth makes the target behind tabCtx answer proxy auth challenges
// with the configured credentials. Fetch events are session-scoped — chromedp
// delivers them only to ListenTarget listeners on the session that enabled the
// domain — so this must run once per target: the initial tab at launch/attach
// and every managed tab via tabSetup. A ListenBrowser listener never sees them.
//
// pauseSuppressed (optional) quiets this listener's blanket ContinueRequest
// while another Fetch user (RouteManager rules, the credentials handler) owns
// request-pause dispatch on the tab — chromedp listeners are additive, so
// without it both would race on every paused request. Auth challenges are
// never suppressed. Never log the password — use p.Redacted().
func EnableProxyAuth(tabCtx context.Context, p config.BrowserProxyConfig, pauseSuppressed *atomic.Bool) error {
	if !ProxyAuthEnabled(p) {
		return nil
	}

	username := p.Username
	password := p.Password

	// Register before Fetch.enable: events cannot flow until the domain is
	// enabled, so nothing is missed, while the reverse order can drop the
	// first authRequired. ListenTarget queues the listener when the target
	// is not materialized yet. Handlers run in goroutines — issuing CDP
	// commands synchronously from the event-dispatch goroutine deadlocks.
	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			go handleAuthRequired(tabCtx, e, username, password)
		case *fetch.EventRequestPaused:
			if pauseSuppressed != nil && pauseSuppressed.Load() {
				return
			}
			go handleRequestPaused(tabCtx, e)
		}
	})

	if err := chromedp.Run(tabCtx,
		fetch.Enable().WithHandleAuthRequests(true),
	); err != nil {
		return fmt.Errorf("enable fetch domain for proxy auth: %w", err)
	}
	return nil
}

// AuthChallengeResponse decides how to answer an auth challenge: provide the
// proxy credentials for proxy challenges, yield with Default for everything
// else (e.g. server WWW-Authenticate).
func AuthChallengeResponse(e *fetch.EventAuthRequired, username, password string) *fetch.AuthChallengeResponse {
	if e == nil || e.AuthChallenge == nil || e.AuthChallenge.Source != fetch.AuthChallengeSourceProxy {
		return &fetch.AuthChallengeResponse{Response: fetch.AuthChallengeResponseResponseDefault}
	}
	return &fetch.AuthChallengeResponse{
		Response: fetch.AuthChallengeResponseResponseProvideCredentials,
		Username: username,
		Password: password,
	}
}

func handleAuthRequired(tabCtx context.Context, e *fetch.EventAuthRequired, username, password string) {
	resp := AuthChallengeResponse(e, username, password)
	if err := chromedp.Run(tabCtx,
		fetch.ContinueWithAuth(e.RequestID, resp),
	); err != nil && tabCtx.Err() == nil {
		slog.Warn("proxy auth: continue with auth failed", "err", err)
	}
}

func handleRequestPaused(tabCtx context.Context, e *fetch.EventRequestPaused) {
	if err := chromedp.Run(tabCtx, fetch.ContinueRequest(e.RequestID)); err != nil && tabCtx.Err() == nil {
		slog.Warn("proxy auth: continue request failed", "err", err)
	}
}
