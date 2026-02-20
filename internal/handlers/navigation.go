package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
)

const maxBodySize = 1 << 20

func (h *Handlers) HandleNavigate(w http.ResponseWriter, r *http.Request) {
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
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.URL == "" {
		web.Error(w, 400, fmt.Errorf("url required"))
		return
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
	if req.BlockMedia != nil && *req.BlockMedia {
		blockPatterns = bridge.MediaBlockPatterns
	} else if req.BlockImages != nil && *req.BlockImages {
		blockPatterns = bridge.ImageBlockPatterns
	} else if req.BlockImages != nil && !*req.BlockImages {
		blockPatterns = nil
	} else if h.Config.BlockMedia {
		blockPatterns = bridge.MediaBlockPatterns
	} else if h.Config.BlockImages {
		blockPatterns = bridge.ImageBlockPatterns
	}

	if req.NewTab {
		newTargetID, newCtx, _, err := h.Bridge.CreateTab(req.URL)
		if err != nil {
			web.Error(w, 500, fmt.Errorf("new tab: %w", err))
			return
		}

		tCtx, tCancel := context.WithTimeout(newCtx, navTimeout)
		defer tCancel()
		go web.CancelOnClientDone(r.Context(), tCancel)

		if blockPatterns != nil {
			_ = bridge.SetResourceBlocking(tCtx, blockPatterns)
		}

		var url string
		_ = chromedp.Run(tCtx, chromedp.Location(&url))
		title := bridge.WaitForTitle(tCtx, titleWait)

		targetID := ""
		if c := chromedp.FromContext(newCtx); c != nil && c.Target != nil {
			targetID = string(c.Target.TargetID)
		} else {
			targetID = newTargetID
		}

		web.JSON(w, 200, map[string]any{"tabId": targetID, "url": url, "title": title})
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

	if blockPatterns != nil {
		_ = bridge.SetResourceBlocking(tCtx, blockPatterns)
	} else if h.Config.BlockImages {
		_ = bridge.SetResourceBlocking(tCtx, nil)
	}

	if err := bridge.NavigatePage(tCtx, req.URL); err != nil {
		web.Error(w, 500, fmt.Errorf("navigate: %w", err))
		return
	}

	h.Bridge.DeleteRefCache(resolvedTabID)

	var url string
	_ = chromedp.Run(tCtx, chromedp.Location(&url))
	title := bridge.WaitForTitle(tCtx, titleWait)

	web.JSON(w, 200, map[string]any{"url": url, "title": title})
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
		newTargetID, ctx, _, err := h.Bridge.CreateTab(req.URL)
		if err != nil {
			web.Error(w, 500, err)
			return
		}

		var curURL, title string
		_ = chromedp.Run(ctx, chromedp.Location(&curURL), chromedp.Title(&title))
		web.JSON(w, 200, map[string]any{"tabId": newTargetID, "url": curURL, "title": title})

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
