package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) handleGetCookies(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	url := r.URL.Query().Get("url")
	name := r.URL.Query().Get("name")

	ctx, _, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	var cookies []*network.Cookie
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if url == "" {
				_ = chromedp.Location(&url).Do(ctx)
			}

			var err error
			cookies, err = network.GetCookies().WithURLs([]string{url}).Do(ctx)
			return err
		}),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("get cookies: %w", err))
		return
	}

	if name != "" {
		filtered := make([]*network.Cookie, 0)
		for _, c := range cookies {
			if c.Name == name {
				filtered = append(filtered, c)
			}
		}
		cookies = filtered
	}

	result := make([]map[string]any, len(cookies))
	for i, c := range cookies {
		result[i] = map[string]any{
			"name":     c.Name,
			"value":    c.Value,
			"domain":   c.Domain,
			"path":     c.Path,
			"secure":   c.Secure,
			"httpOnly": c.HTTPOnly,
			"sameSite": c.SameSite.String(),
		}
		if c.Expires > 0 {
			result[i]["expires"] = c.Expires
		}
	}

	jsonResp(w, 200, map[string]any{
		"url":     url,
		"cookies": result,
		"count":   len(result),
	})
}

type cookieRequest struct {
	TabID   string             `json:"tabId"`
	URL     string             `json:"url"`
	Cookies []cookieSetRequest `json:"cookies"`
}

type cookieSetRequest struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
	Expires  float64 `json:"expires"`
}

func (b *Bridge) handleSetCookies(w http.ResponseWriter, r *http.Request) {
	var req cookieRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.URL == "" {
		jsonErr(w, 400, fmt.Errorf("url is required"))
		return
	}

	if len(req.Cookies) == 0 {
		jsonErr(w, 400, fmt.Errorf("cookies array is empty"))
		return
	}

	ctx, _, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	successCount := 0
	for _, cookie := range req.Cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}

		params := network.SetCookie(cookie.Name, cookie.Value).
			WithURL(req.URL).
			WithHTTPOnly(cookie.HTTPOnly).
			WithSecure(cookie.Secure)

		if cookie.Domain != "" {
			params = params.WithDomain(cookie.Domain)
		}
		if cookie.Path != "" {
			params = params.WithPath(cookie.Path)
		}
		if cookie.Expires > 0 {
			expires := cdp.TimeSinceEpoch(time.Unix(int64(cookie.Expires), 0))
			params = params.WithExpires(&expires)
		}

		if cookie.SameSite != "" {
			var sameSite network.CookieSameSite
			switch strings.ToLower(cookie.SameSite) {
			case "strict":
				sameSite = network.CookieSameSiteStrict
			case "lax":
				sameSite = network.CookieSameSiteLax
			case "none":
				sameSite = network.CookieSameSiteNone
			}
			if sameSite != "" {
				params = params.WithSameSite(sameSite)
			}
		}

		if err := chromedp.Run(tCtx, params); err == nil {
			successCount++
		}
	}

	jsonResp(w, 200, map[string]any{
		"set":    successCount,
		"failed": len(req.Cookies) - successCount,
		"total":  len(req.Cookies),
	})
}
