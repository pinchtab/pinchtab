package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// EnableFetchWithAuth enables the CDP Fetch domain with auth request
// interception, used by the credentials handler.
func (b *Bridge) EnableFetchWithAuth(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().WithHandleAuthRequests(true).Do(ctx)
	}))
}

// DisableFetch disables the CDP Fetch domain.
func (b *Bridge) DisableFetch(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Disable().Do(ctx)
	}))
}

// ListenAuthRequired installs a CDP event listener for Fetch.AuthRequired and
// Fetch.RequestPaused events. The handler callback receives (requestID, isAuth)
// where isAuth=true for AuthRequired events and false for RequestPaused events.
func (b *Bridge) ListenAuthRequired(ctx context.Context, handler func(requestID string, isAuth bool)) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			handler(string(e.RequestID), true)
		case *fetch.EventRequestPaused:
			handler(string(e.RequestID), false)
		}
	})
}

// ContinueWithAuth responds to an auth challenge with credentials.
func (b *Bridge) ContinueWithAuth(ctx context.Context, requestID, username, password string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(innerCtx context.Context) error {
		return fetch.ContinueWithAuth(fetch.RequestID(requestID), &fetch.AuthChallengeResponse{
			Response: fetch.AuthChallengeResponseResponseProvideCredentials,
			Username: username,
			Password: password,
		}).Do(innerCtx)
	}))
}

// ContinueRequest continues a paused request without modification.
func (b *Bridge) ContinueRequest(ctx context.Context, requestID string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(innerCtx context.Context) error {
		return fetch.ContinueRequest(fetch.RequestID(requestID)).Do(innerCtx)
	}))
}

// GoBack navigates back in the browser history. Returns didNavigate=false if
// there is no prior history entry.
func (b *Bridge) GoBack(ctx context.Context) (bool, error) {
	var didNavigate bool
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cur, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}
		if cur <= 0 || cur > int64(len(entries)-1) {
			didNavigate = false
			return nil
		}
		didNavigate = true
		return page.NavigateToHistoryEntry(entries[cur-1].ID).Do(ctx)
	}))
	return didNavigate, err
}

// GoForward navigates forward in the browser history. Returns didNavigate=false
// if there is no forward history entry.
func (b *Bridge) GoForward(ctx context.Context) (bool, error) {
	var didNavigate bool
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cur, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}
		if cur < 0 || cur >= int64(len(entries)-1) {
			didNavigate = false
			return nil
		}
		didNavigate = true
		return page.NavigateToHistoryEntry(entries[cur+1].ID).Do(ctx)
	}))
	return didNavigate, err
}

// Reload reloads the current page.
func (b *Bridge) Reload(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.Reload().Do(ctx)
	}))
}

// WaitVisible waits until the CSS selector matches a visible element.
func (b *Bridge) WaitVisible(ctx context.Context, selector string) error {
	return chromedp.Run(ctx, chromedp.WaitVisible(selector, chromedp.ByQuery))
}

// EnableNetwork enables the CDP Network domain for event listening.
func (b *Bridge) EnableNetwork(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.Enable().Do(ctx)
	}))
}

// ListenNetworkEvents installs a CDP event listener for network request/response
// events, used by the navigation policy guard.
func (b *Bridge) ListenNetworkEvents(ctx context.Context, handler NetworkEventHandler) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			if handler.OnRequestWillBeSent != nil {
				handler.OnRequestWillBeSent(string(e.FrameID), string(e.RequestID), string(e.Type))
			}
		case *network.EventResponseReceived:
			if handler.OnResponseReceived != nil {
				handler.OnResponseReceived(string(e.RequestID), e.Response.RemoteIPAddress)
			}
		}
	})
}

// SetRawCookie sets a cookie via the CDP Network domain. Used by state restore.
func (b *Bridge) SetRawCookie(ctx context.Context, params RawSetCookieParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := network.SetCookie(params.Name, params.Value).
			WithDomain(params.Domain).
			WithPath(params.Path).
			WithSecure(params.Secure).
			WithHTTPOnly(params.HTTPOnly)

		if params.SameSite != "" {
			var sameSite network.CookieSameSite
			switch params.SameSite {
			case "Strict":
				sameSite = network.CookieSameSiteStrict
			case "Lax":
				sameSite = network.CookieSameSiteLax
			case "None":
				sameSite = network.CookieSameSiteNone
			}
			if sameSite != "" {
				p = p.WithSameSite(sameSite)
			}
		}
		return p.Do(ctx)
	}))
}

// GetRawCookies retrieves all cookies from the browser via CDP Network domain.
func (b *Bridge) GetRawCookies(ctx context.Context) ([]RawCookie, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(ctx)
		return err
	}))
	if err != nil {
		return nil, err
	}

	result := make([]RawCookie, len(cookies))
	for i, c := range cookies {
		result[i] = RawCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  float64(c.Expires),
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite.String(),
		}
	}
	return result, nil
}

// SetUserAgentOverride overrides the user agent via CDP Emulation domain.
func (b *Bridge) SetUserAgentOverride(ctx context.Context, params UserAgentOverrideParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := emulation.SetUserAgentOverride(params.UserAgent).WithPlatform(params.Platform)
		if params.AcceptLanguage != "" {
			p = p.WithAcceptLanguage(params.AcceptLanguage)
		}
		return p.Do(ctx)
	}))
}

// SetLocaleOverride overrides the browser locale via CDP Emulation domain.
func (b *Bridge) SetLocaleOverride(ctx context.Context, locale string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetLocaleOverride().WithLocale(locale).Do(ctx)
	}))
}

// SetTimezoneOverride overrides the browser timezone via CDP Emulation domain.
func (b *Bridge) SetTimezoneOverride(ctx context.Context, timezoneID string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetTimezoneOverride(timezoneID).Do(ctx)
	}))
}

// SetDeviceMetricsOverride overrides device metrics (screen size) via CDP Emulation domain.
func (b *Bridge) SetDeviceMetricsOverride(ctx context.Context, params DeviceMetricsOverrideParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(params.Width, params.Height, params.DeviceScaleFactor, params.Mobile).
			WithScreenWidth(params.ScreenWidth).
			WithScreenHeight(params.ScreenHeight).
			Do(ctx)
	}))
}

// AddScriptToEvaluateOnNewDocument adds a script to be evaluated when a new
// document is created, before any page scripts run. Returns the script identifier.
func (b *Bridge) AddScriptToEvaluateOnNewDocument(ctx context.Context, source string) (string, error) {
	var identifier string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		id, err := page.AddScriptToEvaluateOnNewDocument(source).Do(ctx)
		if err != nil {
			return err
		}
		identifier = string(id)
		return nil
	}))
	return identifier, err
}
