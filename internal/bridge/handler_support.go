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

func (b *Bridge) EnableFetchWithAuth(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().WithHandleAuthRequests(true).Do(ctx)
	}))
}

func (b *Bridge) DisableFetch(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Disable().Do(ctx)
	}))
}

// The handler callback receives isAuth=true for AuthRequired events and false
// for RequestPaused events.
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

func (b *Bridge) ContinueWithAuth(ctx context.Context, requestID, username, password string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(innerCtx context.Context) error {
		return fetch.ContinueWithAuth(fetch.RequestID(requestID), &fetch.AuthChallengeResponse{
			Response: fetch.AuthChallengeResponseResponseProvideCredentials,
			Username: username,
			Password: password,
		}).Do(innerCtx)
	}))
}

func (b *Bridge) ContinueRequest(ctx context.Context, requestID string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(innerCtx context.Context) error {
		return fetch.ContinueRequest(fetch.RequestID(requestID)).Do(innerCtx)
	}))
}

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

func (b *Bridge) Reload(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.Reload().Do(ctx)
	}))
}

func (b *Bridge) WaitVisible(ctx context.Context, selector string) error {
	return chromedp.Run(ctx, chromedp.WaitVisible(selector, chromedp.ByQuery))
}

func (b *Bridge) EnableNetwork(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.Enable().Do(ctx)
	}))
}

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

func (b *Bridge) SetUserAgentOverride(ctx context.Context, params UserAgentOverrideParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := emulation.SetUserAgentOverride(params.UserAgent).WithPlatform(params.Platform)
		if params.AcceptLanguage != "" {
			p = p.WithAcceptLanguage(params.AcceptLanguage)
		}
		return p.Do(ctx)
	}))
}

func (b *Bridge) SetLocaleOverride(ctx context.Context, locale string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetLocaleOverride().WithLocale(locale).Do(ctx)
	}))
}

func (b *Bridge) SetTimezoneOverride(ctx context.Context, timezoneID string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetTimezoneOverride(timezoneID).Do(ctx)
	}))
}

func (b *Bridge) SetDeviceMetricsOverride(ctx context.Context, params DeviceMetricsOverrideParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(params.Width, params.Height, params.DeviceScaleFactor, params.Mobile).
			WithScreenWidth(params.ScreenWidth).
			WithScreenHeight(params.ScreenHeight).
			Do(ctx)
	}))
}

// The script is evaluated before any page scripts run.
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
