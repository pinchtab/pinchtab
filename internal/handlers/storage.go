package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (h *Handlers) HandleTabStorageExport(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 15*time.Second)
	defer tCancel()

	var currentURL string
	var cookies []*network.Cookie
	var storage struct {
		Origin         string            `json:"origin"`
		URL            string            `json:"url"`
		LocalStorage   map[string]string `json:"localStorage"`
		SessionStorage map[string]string `json:"sessionStorage"`
	}

	const storageDumpJS = `(() => {
		const toMap = (store) => {
			const out = {};
			for (let i = 0; i < store.length; i++) {
				const key = store.key(i);
				out[key] = store.getItem(key);
			}
			return out;
		};
		return {
			origin: location.origin,
			url: location.href,
			localStorage: toMap(window.localStorage),
			sessionStorage: toMap(window.sessionStorage)
		};
	})()`

	if err := chromedp.Run(tCtx,
		chromedp.Location(&currentURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var getErr error
			cookies, getErr = network.GetCookies().WithURLs([]string{currentURL}).Do(ctx)
			return getErr
		}),
		chromedp.Evaluate(storageDumpJS, &storage),
	); err != nil {
		httpx.ErrorCode(w, 500, "storage_export_failed", fmt.Sprintf("export storage: %v", err), true, nil)
		return
	}

	cookiePayload := make([]map[string]any, 0, len(cookies))
	for _, c := range cookies {
		item := map[string]any{
			"name":     c.Name,
			"value":    c.Value,
			"domain":   c.Domain,
			"path":     c.Path,
			"secure":   c.Secure,
			"httpOnly": c.HTTPOnly,
			"sameSite": c.SameSite.String(),
		}
		if c.Expires > 0 {
			item["expires"] = c.Expires
		}
		cookiePayload = append(cookiePayload, item)
	}

	httpx.JSON(w, 200, map[string]any{
		"tabId":          resolvedTabID,
		"url":            storage.URL,
		"origin":         storage.Origin,
		"cookies":        cookiePayload,
		"localStorage":   storage.LocalStorage,
		"sessionStorage": storage.SessionStorage,
	})
}

func (h *Handlers) HandleTabStorageImport(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	var req struct {
		URL            string             `json:"url"`
		Cookies        []cookieSetRequest `json:"cookies"`
		LocalStorage   map[string]string  `json:"localStorage"`
		SessionStorage map[string]string  `json:"sessionStorage"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 15*time.Second)
	defer tCancel()

	if req.URL == "" {
		if err := chromedp.Run(tCtx, chromedp.Location(&req.URL)); err != nil {
			httpx.ErrorCode(w, 500, "storage_import_failed", fmt.Sprintf("resolve current url: %v", err), true, nil)
			return
		}
	}

	setCookies := 0
	for _, cookie := range req.Cookies {
		if cookie.Name == "" {
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
		if err := chromedp.Run(tCtx, params); err == nil {
			setCookies++
		}
	}

	payload, err := json.Marshal(map[string]any{
		"localStorage":   req.LocalStorage,
		"sessionStorage": req.SessionStorage,
	})
	if err != nil {
		httpx.ErrorCode(w, 500, "storage_import_failed", fmt.Sprintf("encode storage payload: %v", err), false, nil)
		return
	}

	script := fmt.Sprintf(`(() => {
		const payload = %s;
		const apply = (store, values) => {
			if (!values) return 0;
			let count = 0;
			for (const [key, value] of Object.entries(values)) {
				store.setItem(key, String(value));
				count++;
			}
			return count;
		};
		return {
			localStorage: apply(window.localStorage, payload.localStorage),
			sessionStorage: apply(window.sessionStorage, payload.sessionStorage)
		};
	})();`, string(payload))

	var storageCounts struct {
		LocalStorage   int `json:"localStorage"`
		SessionStorage int `json:"sessionStorage"`
	}
	if err := chromedp.Run(tCtx, chromedp.Evaluate(script, &storageCounts)); err != nil {
		httpx.ErrorCode(w, 500, "storage_import_failed", fmt.Sprintf("hydrate storage: %v", err), true, nil)
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"tabId":             resolvedTabID,
		"url":               req.URL,
		"setCookies":        setCookies,
		"setLocalStorage":   storageCounts.LocalStorage,
		"setSessionStorage": storageCounts.SessionStorage,
	})
}
