package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// ── GET /health ────────────────────────────────────────────

func (b *Bridge) handleHealth(w http.ResponseWriter, r *http.Request) {
	targets, err := b.ListTargets()
	if err != nil {
		jsonResp(w, 200, map[string]any{"status": "disconnected", "error": err.Error(), "cdp": cdpURL})
		return
	}
	jsonResp(w, 200, map[string]any{"status": "ok", "tabs": len(targets), "cdp": cdpURL})
}

// ── GET /tabs ──────────────────────────────────────────────

func (b *Bridge) handleTabs(w http.ResponseWriter, r *http.Request) {
	targets, err := b.ListTargets()
	if err != nil {
		jsonErr(w, 500, err)
		return
	}

	tabs := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		tabs = append(tabs, map[string]any{
			"id":    string(t.TargetID),
			"url":   t.URL,
			"title": t.Title,
			"type":  t.Type,
		})
	}
	jsonResp(w, 200, map[string]any{"tabs": tabs})
}

// ── GET /snapshot ──────────────────────────────────────────

func (b *Bridge) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	filter := r.URL.Query().Get("filter")
	maxDepthStr := r.URL.Query().Get("depth")
	maxDepth := -1
	if maxDepthStr != "" {
		if d, err := strconv.Atoi(maxDepthStr); err == nil {
			maxDepth = d
		}
	}

	ctx, resolvedTabID, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var rawResult json.RawMessage
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Accessibility.getFullAXTree", nil, &rawResult)
		}),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("a11y tree: %w", err))
		return
	}

	var treeResp struct {
		Nodes []rawAXNode `json:"nodes"`
	}
	if err := json.Unmarshal(rawResult, &treeResp); err != nil {
		jsonErr(w, 500, fmt.Errorf("parse a11y tree: %w", err))
		return
	}

	flat, refs := buildSnapshot(treeResp.Nodes, filter, maxDepth)

	// Cache ref→nodeID mapping for this tab
	b.mu.Lock()
	b.snapshots[resolvedTabID] = &refCache{refs: refs}
	b.mu.Unlock()

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	jsonResp(w, 200, map[string]any{
		"url":   url,
		"title": title,
		"nodes": flat,
		"count": len(flat),
	})
}

// ── GET /screenshot ────────────────────────────────────────

func (b *Bridge) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")

	ctx, _, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var buf []byte
	quality := 80
	if q := r.URL.Query().Get("quality"); q != "" {
		if qn, err := strconv.Atoi(q); err == nil {
			quality = qn
		}
	}

	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(int64(quality)).
				Do(ctx)
			return err
		}),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("screenshot: %w", err))
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "image/jpeg")
		if _, err := w.Write(buf); err != nil {
			slog.Error("screenshot write", "err", err)
		}
		return
	}

	jsonResp(w, 200, map[string]any{
		"format": "jpeg",
		"base64": buf,
	})
}

// ── GET /text ──────────────────────────────────────────────

func (b *Bridge) handleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")

	ctx, _, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var text string
	if err := chromedp.Run(tCtx,
		chromedp.Evaluate(`document.body.innerText`, &text),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("text extract: %w", err))
		return
	}

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	jsonResp(w, 200, map[string]any{
		"url":   url,
		"title": title,
		"text":  text,
	})
}

// ── POST /navigate ─────────────────────────────────────────

func (b *Bridge) handleNavigate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID string `json:"tabId"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.URL == "" {
		jsonResp(w, 400, map[string]string{"error": "url required"})
		return
	}

	ctx, resolvedTabID, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, navigateTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	// Use raw CDP navigate + WaitReady instead of chromedp.Navigate
	// which waits for the full load event (never fires on SPAs)
	if err := navigatePage(tCtx, req.URL); err != nil {
		jsonErr(w, 500, fmt.Errorf("navigate: %w", err))
		return
	}

	b.mu.Lock()
	delete(b.snapshots, resolvedTabID)
	b.mu.Unlock()

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	jsonResp(w, 200, map[string]any{"url": url, "title": title})
}

// ── POST /action ───────────────────────────────────────────

// actionRequest is the parsed JSON body for /action.
type actionRequest struct {
	TabID    string `json:"tabId"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Key      string `json:"key"`
	NodeID   int64  `json:"nodeId"`
}

// ActionFunc handles a single action kind. Receives the full request for
// clean access to all fields without parameter fragmentation.
type ActionFunc func(ctx context.Context, req actionRequest) (map[string]any, error)

func (b *Bridge) actionRegistry() map[string]ActionFunc {
	return map[string]ActionFunc{
		actionClick: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"clicked": true}, chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery))
			}
			if req.NodeID > 0 {
				return map[string]any{"clicked": true}, clickByNodeID(ctx, req.NodeID)
			}
			return nil, fmt.Errorf("need selector, ref, or nodeId")
		},
		actionType: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Text == "" {
				return nil, fmt.Errorf("text required for type")
			}
			if req.Selector != "" {
				return map[string]any{"typed": req.Text}, chromedp.Run(ctx,
					chromedp.Click(req.Selector, chromedp.ByQuery),
					chromedp.SendKeys(req.Selector, req.Text, chromedp.ByQuery),
				)
			}
			if req.NodeID > 0 {
				return map[string]any{"typed": req.Text}, typeByNodeID(ctx, req.NodeID, req.Text)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		actionFill: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"filled": req.Text}, chromedp.Run(ctx, chromedp.SetValue(req.Selector, req.Text, chromedp.ByQuery))
			}
			return map[string]any{"filled": req.Text}, nil
		},
		actionPress: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Key == "" {
				return nil, fmt.Errorf("key required for press")
			}
			return map[string]any{"pressed": req.Key}, chromedp.Run(ctx, chromedp.KeyEvent(req.Key))
		},
		actionFocus: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"focused": true}, chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery))
			}
			if req.NodeID > 0 {
				return map[string]any{"focused": true}, chromedp.Run(ctx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						p := map[string]any{"backendNodeId": req.NodeID}
						return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", p, nil)
					}),
				)
			}
			return map[string]any{"focused": true}, nil
		},
	}
}

func (b *Bridge) handleAction(w http.ResponseWriter, r *http.Request) {
	var req actionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, resolvedTabID, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	// Resolve ref to backendNodeID from cached snapshot
	if req.Ref != "" && req.NodeID == 0 && req.Selector == "" {
		b.mu.RLock()
		cache := b.snapshots[resolvedTabID]
		b.mu.RUnlock()
		if cache != nil {
			if nid, ok := cache.refs[req.Ref]; ok {
				req.NodeID = nid
			}
		}
		if req.NodeID == 0 {
			jsonResp(w, 400, map[string]string{
				"error": fmt.Sprintf("ref %s not found — take a /snapshot first", req.Ref),
			})
			return
		}
	}

	fn, ok := b.actionRegistry()[req.Kind]
	if !ok {
		jsonResp(w, 400, map[string]string{"error": fmt.Sprintf("unknown action: %s", req.Kind)})
		return
	}

	result, err := fn(tCtx, req)
	if err != nil {
		jsonErr(w, 500, fmt.Errorf("action %s: %w", req.Kind, err))
		return
	}

	jsonResp(w, 200, result)
}

// ── POST /evaluate ─────────────────────────────────────────

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
		jsonResp(w, 400, map[string]string{"error": "expression required"})
		return
	}

	ctx, _, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var result any
	if err := chromedp.Run(tCtx,
		chromedp.Evaluate(req.Expression, &result),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("evaluate: %w", err))
		return
	}

	jsonResp(w, 200, map[string]any{"result": result})
}

// ── POST /tab ──────────────────────────────────────────────

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
		ctx, cancel := chromedp.NewContext(b.browserCtx)

		url := "about:blank"
		if req.URL != "" {
			url = req.URL
		}
		if err := navigatePage(ctx, url); err != nil {
			cancel()
			jsonErr(w, 500, fmt.Errorf("new tab: %w", err))
			return
		}

		newTargetID := string(chromedp.FromContext(ctx).Target.TargetID)
		b.mu.Lock()
		b.tabs[newTargetID] = &TabEntry{ctx: ctx, cancel: cancel}
		b.mu.Unlock()

		var curURL, title string
		_ = chromedp.Run(ctx, chromedp.Location(&curURL), chromedp.Title(&title))
		jsonResp(w, 200, map[string]any{"tabId": newTargetID, "url": curURL, "title": title})

	case tabActionClose:
		if req.TabID == "" {
			jsonResp(w, 400, map[string]string{"error": "tabId required"})
			return
		}

		b.mu.Lock()
		if entry, ok := b.tabs[req.TabID]; ok {
			if entry.cancel != nil {
				entry.cancel()
			}
			delete(b.tabs, req.TabID)
			delete(b.snapshots, req.TabID)
		}
		b.mu.Unlock()

		ctx, cancel := chromedp.NewContext(b.browserCtx,
			chromedp.WithTargetID(target.ID(req.TabID)),
		)
		defer cancel()
		if err := chromedp.Run(ctx, page.Close()); err != nil {
			jsonErr(w, 500, fmt.Errorf("close tab: %w", err))
			return
		}
		jsonResp(w, 200, map[string]any{"closed": true})

	default:
		jsonResp(w, 400, map[string]string{"error": "action must be 'new' or 'close'"})
	}
}
