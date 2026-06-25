package bridge

import (
	"context"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) GetCookies(ctx context.Context, urls []string) ([]CookieData, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().WithURLs(urls).Do(ctx)
		return err
	}))
	if err != nil {
		return nil, err
	}

	result := make([]CookieData, len(cookies))
	for i, c := range cookies {
		result[i] = CookieData{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  float64(c.Expires),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: c.SameSite.String(),
		}
	}
	return result, nil
}

func (b *Bridge) SetCookie(ctx context.Context, params SetCookieParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := network.SetCookie(params.Name, params.Value).
			WithURL(params.URL).
			WithHTTPOnly(params.HTTPOnly).
			WithSecure(params.Secure)

		if params.Domain != "" {
			p = p.WithDomain(params.Domain)
		}
		if params.Path != "" {
			p = p.WithPath(params.Path)
		}
		if params.Expires > 0 {
			expires := cdp.TimeSinceEpoch(time.Unix(int64(params.Expires), 0))
			p = p.WithExpires(&expires)
		}
		if params.SameSite != "" {
			var sameSite network.CookieSameSite
			switch strings.ToLower(params.SameSite) {
			case "strict":
				sameSite = network.CookieSameSiteStrict
			case "lax":
				sameSite = network.CookieSameSiteLax
			case "none":
				sameSite = network.CookieSameSiteNone
			}
			if sameSite != "" {
				p = p.WithSameSite(sameSite)
			}
		}

		return p.Do(ctx)
	}))
}
