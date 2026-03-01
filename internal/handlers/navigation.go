package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
)

const maxBodySize = 1 << 20

// HandleNavigate navigates a tab to a URL or creates a new tab.
//
// @Endpoint POST /navigate
// @Description Navigate to a URL in an existing tab or create a new tab and navigate
//
// @Param tabId string body Tab ID to navigate in (optional - creates new if omitted)
// @Param url string body URL to navigate to (required)
// @Param newTab bool body Force create new tab (optional, default: false)
// @Param waitTitle float64 body Wait for title change (ms) (optional, default: 0)
// @Param timeout float64 body Timeout for navigation (ms) (optional, default: 30000)
//
// @Response 200 application/json Returns {tabId, url, title}
// @Response 400 application/json Invalid URL or parameters
// @Response 500 application/json Chrome error
//
// @Example curl navigate new:
//
//	curl -X POST http://localhost:9867/navigate \
//	  -H "Content-Type: application/json" \
//	  -d '{"url":"https://example.com"}'
//
// @Example curl navigate existing:
//
//	curl -X POST http://localhost:9867/navigate \
//	  -H "Content-Type: application/json" \
//	  -d '{"tabId":"abc123","url":"https://google.com"}'
//
// @Example cli:
//
//	pinchtab nav https://example.com
func (h *Handlers) HandleNavigate(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req struct {
		TabID       string  `json:"tabId"`
		URL         string  `json:"url"`
		NewTab      bool    `json:"newTab"`
		WaitTitle   float64 `json:"waitTitle"`
		Timeout     float64 `json:"timeout"`
		BlockImages *bool   `json:"blockImages"`
		BlockMedia  *bool   `json:"blockMedia"`
		BlockAds    *bool   `json:"blockAds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.URL == "" {
		web.Error(w, 400, fmt.Errorf("url required"))
		return
	}

	// Default to creating new tab (API design: /navigate always creates new tab)
	// Unless explicitly reusing an existing tab by specifying TabID
	if req.TabID == "" {
		req.NewTab = true
	}

	titleWait := time.Duration(0)
	if req.WaitTitle > 0 {
		if req.WaitTitle > 30 {
			req.WaitTitle = 30
		}
		titleWait = time.Duration(req.WaitTitle * float64(time.Second))
	}

	navTimeout := h.Config.NavigateTimeout
	if req.Timeout > 0 {
		if req.Timeout > 120 {
			req.Timeout = 120
		}
		navTimeout = time.Duration(req.Timeout * float64(time.Second))
	}

	var blockPatterns []string

	blockAds := h.Config.BlockAds
	if req.BlockAds != nil {
		blockAds = *req.BlockAds
	}

	blockMedia := h.Config.BlockMedia
	if req.BlockMedia != nil {
		blockMedia = *req.BlockMedia
	}

	blockImages := h.Config.BlockImages
	if req.BlockImages != nil {
		blockImages = *req.BlockImages
	}

	if blockAds {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.AdBlockPatterns)
	}

	if blockMedia {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.MediaBlockPatterns)
	} else if blockImages {
		blockPatterns = bridge.CombineBlockPatterns(blockPatterns, bridge.ImageBlockPatterns)
	}

	if req.NewTab {
		// Block dangerous/unsupported schemes; allow bare hostnames (e.g. "example.com")
		// which Chrome handles gracefully by prepending https://.
		if parsed, err := url.Parse(req.URL); err == nil && parsed.Scheme != "" {
			blocked := parsed.Scheme == "javascript" || parsed.Scheme == "vbscript" || parsed.Scheme == "data"
			if blocked {
				web.Error(w, 400, fmt.Errorf("invalid URL scheme: %s", parsed.Scheme))
				return
			}
		}
		// CreateTab returns hash-based tab ID directly (e.g., "tab_XXXXXXXX")
		hashTabID, newCtx, _, err := h.Bridge.CreateTab(req.URL)
		if err != nil {
			web.Error(w, 500, fmt.Errorf("new tab: %w", err))
			return
		}

		tCtx, tCancel := context.WithTimeout(newCtx, navTimeout)
		defer tCancel()
		go web.CancelOnClientDone(r.Context(), tCancel)

		if len(blockPatterns) > 0 {
			_ = bridge.SetResourceBlocking(tCtx, blockPatterns)
		}

		if err := bridge.NavigatePage(tCtx, req.URL); err != nil {
			code := 500
			errMsg := err.Error()
			if strings.Contains(errMsg, "invalid URL") || strings.Contains(errMsg, "Cannot navigate to invalid URL") || strings.Contains(errMsg, "ERR_INVALID_URL") {
				code = 400
			}
			web.Error(w, code, fmt.Errorf("navigate: %w", err))
			return
		}

		var url string
		_ = chromedp.Run(tCtx, chromedp.Location(&url))
		title := bridge.WaitForTitle(tCtx, titleWait)

		web.JSON(w, 200, map[string]any{"tabId": hashTabID, "url": url, "title": title})
		return
	}

	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, navTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	if len(blockPatterns) > 0 {
		_ = bridge.SetResourceBlocking(tCtx, blockPatterns)
	} else {
		// Clear any existing blocking patterns
		_ = bridge.SetResourceBlocking(tCtx, nil)
	}

	if err := bridge.NavigatePage(tCtx, req.URL); err != nil {
		code := 500
		errMsg := err.Error()
		if strings.Contains(errMsg, "invalid URL") || strings.Contains(errMsg, "Cannot navigate to invalid URL") || strings.Contains(errMsg, "ERR_INVALID_URL") {
			code = 400
		}
		web.Error(w, code, fmt.Errorf("navigate: %w", err))
		return
	}

	h.Bridge.DeleteRefCache(resolvedTabID)

	var url string
	_ = chromedp.Run(tCtx, chromedp.Location(&url))
	title := bridge.WaitForTitle(tCtx, titleWait)

	web.JSON(w, 200, map[string]any{"tabId": resolvedTabID, "url": url, "title": title})
}

// HandleTabNavigate navigates an existing tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/navigate
func (h *Handlers) HandleTabNavigate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	if r.Body != nil {
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&body)
		if err != nil && !errors.Is(err, io.EOF) {
			web.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			web.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			web.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	// Path tab ID is canonical for this endpoint and always navigates existing tab.
	body["tabId"] = tabID
	body["newTab"] = false

	payload, err := json.Marshal(body)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.HandleNavigate(w, req)
}

func (h *Handlers) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID      string `json:"tabId"`
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Expression == "" {
		web.Error(w, 400, fmt.Errorf("expression required"))
		return
	}

	ctx, _, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	var result any
	if err := chromedp.Run(tCtx, chromedp.Evaluate(req.Expression, &result)); err != nil {
		web.Error(w, 500, fmt.Errorf("evaluate: %w", err))
		return
	}

	web.JSON(w, 200, map[string]any{"result": result})
}

// HandleTabEvaluate runs JavaScript in a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/evaluate
func (h *Handlers) HandleTabEvaluate(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			web.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			web.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	body["tabId"] = tabID

	payload, err := json.Marshal(body)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.HandleEvaluate(w, req)
}

const (
	tabActionNew   = "new"
	tabActionClose = "close"
)

func (h *Handlers) HandleTab(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		TabID  string `json:"tabId"`
		URL    string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	switch req.Action {
	case tabActionNew:
		// CreateTab returns hash-based tab ID directly (e.g., "tab_XXXXXXXX")
		hashTabID, ctx, _, err := h.Bridge.CreateTab(req.URL)
		if err != nil {
			web.Error(w, 500, err)
			return
		}

		if req.URL != "" && req.URL != "about:blank" {
			tCtx, tCancel := context.WithTimeout(ctx, h.Config.NavigateTimeout)
			defer tCancel()
			if err := bridge.NavigatePage(tCtx, req.URL); err != nil {
				_ = h.Bridge.CloseTab(hashTabID)
				web.Error(w, 500, fmt.Errorf("navigate: %w", err))
				return
			}
		}

		var curURL, title string
		_ = chromedp.Run(ctx, chromedp.Location(&curURL), chromedp.Title(&title))

		web.JSON(w, 200, map[string]any{"tabId": hashTabID, "url": curURL, "title": title})

	case tabActionClose:
		if req.TabID == "" {
			web.Error(w, 400, fmt.Errorf("tabId required"))
			return
		}

		if err := h.Bridge.CloseTab(req.TabID); err != nil {
			web.Error(w, 500, err)
			return
		}
		web.JSON(w, 200, map[string]any{"closed": true})

	default:
		web.Error(w, 400, fmt.Errorf("action must be 'new' or 'close'"))
	}
}
