package bridge

import (
	"context"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// CookieData is a bridge-level representation of a browser cookie,
// avoiding direct use of cdproto types in handler code.
type CookieData struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite"`
}

// SetCookieParams holds parameters for setting a single cookie.
type SetCookieParams struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	URL      string  `json:"url"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
	Expires  float64 `json:"expires"`
}

// GetCookies retrieves cookies for the given URLs via CDP.
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

// SetCookie sets a single cookie via CDP.
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
