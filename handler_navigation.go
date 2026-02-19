package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
)

func (b *Bridge) handleNavigate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID       string  `json:"tabId"`
		URL         string  `json:"url"`
		NewTab      bool    `json:"newTab"`
		WaitTitle   float64 `json:"waitTitle"`
		Timeout     float64 `json:"timeout"`
		BlockImages *bool   `json:"blockImages"`
		BlockMedia  *bool   `json:"blockMedia"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.URL == "" {
		jsonErr(w, 400, fmt.Errorf("url required"))
		return
	}

	titleWait := time.Duration(0)
	if req.WaitTitle > 0 {
		if req.WaitTitle > 30 {
			req.WaitTitle = 30
		}
		titleWait = time.Duration(req.WaitTitle * float64(time.Second))
	}

	navTimeout := cfg.NavigateTimeout
	if req.Timeout > 0 {
		if req.Timeout > 120 {
			req.Timeout = 120
		}
		navTimeout = time.Duration(req.Timeout * float64(time.Second))
	}

	var blockPatterns []string
	if req.BlockMedia != nil && *req.BlockMedia {
		blockPatterns = mediaBlockPatterns
	} else if req.BlockImages != nil && *req.BlockImages {
		blockPatterns = imageBlockPatterns
	} else if req.BlockImages != nil && !*req.BlockImages {
		blockPatterns = nil
	} else if cfg.BlockMedia {
		blockPatterns = mediaBlockPatterns
	} else if cfg.BlockImages {
		blockPatterns = imageBlockPatterns
	}

	if req.NewTab {
		newTargetID, newCtx, _, err := b.CreateTab(req.URL)
		if err != nil {
			jsonErr(w, 500, fmt.Errorf("new tab: %w", err))
			return
		}

		tCtx, tCancel := context.WithTimeout(newCtx, navTimeout)
		defer tCancel()
		go cancelOnClientDone(r.Context(), tCancel)

		if blockPatterns != nil {
			_ = setResourceBlocking(tCtx, blockPatterns)
		}

		var url, title string
		_ = chromedp.Run(tCtx, chromedp.Location(&url))
		title = waitForTitle(tCtx, titleWait)

		jsonResp(w, 200, map[string]any{"tabId": newTargetID, "url": url, "title": title})
		return
	}

	ctx, resolvedTabID, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, navTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	if blockPatterns != nil {
		_ = setResourceBlocking(tCtx, blockPatterns)
	} else if cfg.BlockImages {
		_ = setResourceBlocking(tCtx, nil)
	}

	if err := navigatePage(tCtx, req.URL); err != nil {
		jsonErr(w, 500, fmt.Errorf("navigate: %w", err))
		return
	}

	b.DeleteRefCache(resolvedTabID)

	var url string
	_ = chromedp.Run(tCtx, chromedp.Location(&url))
	title := waitForTitle(tCtx, titleWait)

	jsonResp(w, 200, map[string]any{"url": url, "title": title})
}

func (b *Bridge) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID      string `json:"tabId"`
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Expression == "" {
		jsonErr(w, 400, fmt.Errorf("expression required"))
		return
	}

	ctx, _, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, cfg.ActionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var result any
	if err := chromedp.Run(tCtx, chromedp.Evaluate(req.Expression, &result)); err != nil {
		jsonErr(w, 500, fmt.Errorf("evaluate: %w", err))
		return
	}

	jsonResp(w, 200, map[string]any{"result": result})
}

func (b *Bridge) handleTab(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		TabID  string `json:"tabId"`
		URL    string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	switch req.Action {
	case tabActionNew:
		newTargetID, ctx, _, err := b.CreateTab(req.URL)
		if err != nil {
			jsonErr(w, 500, err)
			return
		}

		var curURL, title string
		_ = chromedp.Run(ctx, chromedp.Location(&curURL), chromedp.Title(&title))
		jsonResp(w, 200, map[string]any{"tabId": newTargetID, "url": curURL, "title": title})

	case tabActionClose:
		if req.TabID == "" {
			jsonErr(w, 400, fmt.Errorf("tabId required"))
			return
		}

		if err := b.CloseTab(req.TabID); err != nil {
			jsonErr(w, 500, err)
			return
		}
		jsonResp(w, 200, map[string]any{"closed": true})

	default:
		jsonErr(w, 400, fmt.Errorf("action must be 'new' or 'close'"))
	}
}
