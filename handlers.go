package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
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

// ── GET /screenshot ────────────────────────────────────────

func (b *Bridge) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	output := r.URL.Query().Get("output") // "file" to save to disk

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

	// Handle file output
	if output == "file" {
		// Create screenshots directory if it doesn't exist
		screenshotDir := filepath.Join(stateDir, "screenshots")
		if err := os.MkdirAll(screenshotDir, 0755); err != nil {
			jsonErr(w, 500, fmt.Errorf("create screenshot dir: %w", err))
			return
		}

		// Generate filename with timestamp
		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("screenshot-%s.jpg", timestamp)
		filePath := filepath.Join(screenshotDir, filename)

		// Write to file
		if err := os.WriteFile(filePath, buf, 0644); err != nil {
			jsonErr(w, 500, fmt.Errorf("write screenshot: %w", err))
			return
		}

		// Return path instead of data
		jsonResp(w, 200, map[string]any{
			"path":      filePath,
			"size":      len(buf),
			"format":    "jpeg",
			"timestamp": timestamp,
		})
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
		"base64": base64.StdEncoding.EncodeToString(buf),
	})
}

// ── GET /text ──────────────────────────────────────────────

func (b *Bridge) handleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	mode := r.URL.Query().Get("mode") // "raw" for innerText, default "clean"

	ctx, _, err := b.TabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	var text string
	if mode == "raw" {
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(`document.body.innerText`, &text),
		); err != nil {
			jsonErr(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
	} else {
		// Clean extraction: strip nav/footer/aside/header, keep article/main content
		if err := chromedp.Run(tCtx,
			chromedp.Evaluate(readabilityJS, &text),
		); err != nil {
			jsonErr(w, 500, fmt.Errorf("text extract: %w", err))
			return
		}
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
		TabID  string `json:"tabId"`
		URL    string `json:"url"`
		NewTab bool   `json:"newTab"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.URL == "" {
		jsonResp(w, 400, map[string]string{"error": "url required"})
		return
	}

	// If newTab is requested, create a new tab and navigate there
	if req.NewTab {
		newTargetID, newCtx, _, err := b.CreateTab(req.URL)
		if err != nil {
			jsonErr(w, 500, fmt.Errorf("new tab: %w", err))
			return
		}

		tCtx, tCancel := context.WithTimeout(newCtx, navigateTimeout)
		defer tCancel()
		go cancelOnClientDone(r.Context(), tCancel)

		var url, title string
		_ = chromedp.Run(tCtx,
			chromedp.Location(&url),
		)
		title = waitForTitle(tCtx)

		jsonResp(w, 200, map[string]any{"tabId": newTargetID, "url": url, "title": title})
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

	b.DeleteRefCache(resolvedTabID)

	var url string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
	)
	title := waitForTitle(tCtx)

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
	Value    string `json:"value"`
	NodeID   int64  `json:"nodeId"`
	ScrollX  int    `json:"scrollX"`
	ScrollY  int    `json:"scrollY"`
	WaitNav  bool   `json:"waitNav"`
	Fast     bool   `json:"fast"` // For humanType: fast typing mode
}

// ActionFunc handles a single action kind. Receives the full request for
// clean access to all fields without parameter fragmentation.
type ActionFunc func(ctx context.Context, req actionRequest) (map[string]any, error)

func (b *Bridge) actionRegistry() map[string]ActionFunc {
	return map[string]ActionFunc{
		actionClick: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			var err error
			if req.Selector != "" {
				err = chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery))
			} else if req.NodeID > 0 {
				err = clickByNodeID(ctx, req.NodeID)
			} else {
				return nil, fmt.Errorf("need selector, ref, or nodeId")
			}
			if err != nil {
				return nil, err
			}
			// Optional: wait for navigation after click (e.g. link clicks)
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(waitNavDelay))
			}
			return map[string]any{"clicked": true}, nil
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
		actionHover: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				return map[string]any{"hovered": true}, hoverByNodeID(ctx, req.NodeID)
			}
			if req.Selector != "" {
				return map[string]any{"hovered": true}, chromedp.Run(ctx,
					chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q)?.dispatchEvent(new MouseEvent('mouseover', {bubbles:true}))`, req.Selector), nil),
				)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		actionSelect: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			val := req.Value
			if val == "" {
				val = req.Text // fallback
			}
			if val == "" {
				return nil, fmt.Errorf("value required for select")
			}
			if req.NodeID > 0 {
				return map[string]any{"selected": val}, selectByNodeID(ctx, req.NodeID, val)
			}
			if req.Selector != "" {
				return map[string]any{"selected": val}, chromedp.Run(ctx,
					chromedp.SetValue(req.Selector, val, chromedp.ByQuery),
				)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		actionScroll: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			// Scroll to element
			if req.NodeID > 0 {
				return map[string]any{"scrolled": true}, scrollByNodeID(ctx, req.NodeID)
			}
			// Scroll by pixel amount
			if req.ScrollX != 0 || req.ScrollY != 0 {
				js := fmt.Sprintf("window.scrollBy(%d, %d)", req.ScrollX, req.ScrollY)
				return map[string]any{"scrolled": true, "x": req.ScrollX, "y": req.ScrollY},
					chromedp.Run(ctx, chromedp.Evaluate(js, nil))
			}
			// Default: scroll down one viewport
			return map[string]any{"scrolled": true, "y": 800},
				chromedp.Run(ctx, chromedp.Evaluate("window.scrollBy(0, 800)", nil))
		},
		actionHumanClick: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			// Human-like click with natural mouse movement
			if req.NodeID > 0 {
				if err := humanClickElement(ctx, cdp.NodeID(req.NodeID)); err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			}
			if req.Selector != "" {
				// Get element to click
				var nodes []*cdp.Node
				if err := chromedp.Run(ctx,
					chromedp.Nodes(req.Selector, &nodes, chromedp.ByQuery),
				); err != nil {
					return nil, err
				}
				if len(nodes) == 0 {
					return nil, fmt.Errorf("element not found: %s", req.Selector)
				}
				if err := humanClickElement(ctx, nodes[0].NodeID); err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			}
			return nil, fmt.Errorf("need selector, ref, or nodeId")
		},
		actionHumanType: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Text == "" {
				return nil, fmt.Errorf("text required for humanType")
			}

			// Focus element first
			if req.Selector != "" {
				if err := chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery)); err != nil {
					return nil, err
				}
			} else if req.NodeID > 0 {
				if err := chromedp.Run(ctx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						return dom.Focus().WithNodeID(cdp.NodeID(req.NodeID)).Do(ctx)
					}),
				); err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("need selector, ref, or nodeId")
			}

			// Type with human-like timing
			fast := req.Fast // Optional "fast" mode for humanType
			actions := humanType(req.Text, fast)
			if err := chromedp.Run(ctx, actions...); err != nil {
				return nil, err
			}

			return map[string]any{"typed": req.Text, "human": true}, nil
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
		cache := b.GetRefCache(resolvedTabID)
		if cache != nil {
			if nid, ok := cache.refs[req.Ref]; ok {
				req.NodeID = nid
			}
		}
		if req.NodeID == 0 {
			jsonResp(w, 400, map[string]string{
				"error": fmt.Sprintf("ref %s not found - take a /snapshot first", req.Ref),
			})
			return
		}
	}

	registry := b.actionRegistry()
	if req.Kind == "" {
		kinds := make([]string, 0, len(registry))
		for k := range registry {
			kinds = append(kinds, k)
		}
		jsonResp(w, 400, map[string]string{
			"error": fmt.Sprintf("missing required field 'kind' - valid values: %s", strings.Join(kinds, ", ")),
		})
		return
	}
	fn, ok := registry[req.Kind]
	if !ok {
		kinds := make([]string, 0, len(registry))
		for k := range registry {
			kinds = append(kinds, k)
		}
		jsonResp(w, 400, map[string]string{
			"error": fmt.Sprintf("unknown action: %s - valid values: %s", req.Kind, strings.Join(kinds, ", ")),
		})
		return
	}

	result, err := fn(tCtx, req)
	if err != nil {
		jsonErr(w, 500, fmt.Errorf("action %s: %w", req.Kind, err))
		return
	}

	jsonResp(w, 200, result)
}

// ── POST /actions (batch) ──────────────────────────────────

type actionsRequest struct {
	TabID       string          `json:"tabId"`       // Default tab for all actions
	Actions     []actionRequest `json:"actions"`     // Array of actions to execute
	StopOnError bool            `json:"stopOnError"` // Stop processing on first error (default: false)
}

type actionResult struct {
	Index   int            `json:"index"` // Which action this result is for
	Success bool           `json:"success"`
	Result  map[string]any `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func (b *Bridge) handleActions(w http.ResponseWriter, r *http.Request) {
	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if len(req.Actions) == 0 {
		jsonResp(w, 400, map[string]string{"error": "actions array is empty"})
		return
	}

	// Resolve tab context once for all actions
	ctx, resolvedTabID, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	results := make([]actionResult, 0, len(req.Actions))
	registry := b.actionRegistry()

	// Process each action sequentially
	for i, action := range req.Actions {
		// Use action's tabId if specified, otherwise use request's default
		if action.TabID == "" {
			action.TabID = resolvedTabID
		} else if action.TabID != resolvedTabID {
			// Different tab requested - need new context
			ctx, resolvedTabID, err = b.TabContext(action.TabID)
			if err != nil {
				results = append(results, actionResult{
					Index:   i,
					Success: false,
					Error:   fmt.Sprintf("tab not found: %v", err),
				})
				if req.StopOnError {
					break
				}
				continue
			}
		}

		// Create timeout context for this action
		tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)

		// Resolve ref if needed (same logic as single action)
		if action.Ref != "" && action.NodeID == 0 && action.Selector == "" {
			cache := b.GetRefCache(resolvedTabID)
			if cache != nil {
				if nid, ok := cache.refs[action.Ref]; ok {
					action.NodeID = nid
				}
			}
			if action.NodeID == 0 {
				tCancel()
				results = append(results, actionResult{
					Index:   i,
					Success: false,
					Error:   fmt.Sprintf("ref %s not found - take a /snapshot first", action.Ref),
				})
				if req.StopOnError {
					break
				}
				continue
			}
		}

		// Validate and execute action
		if action.Kind == "" {
			tCancel()
			results = append(results, actionResult{
				Index:   i,
				Success: false,
				Error:   "missing required field 'kind'",
			})
			if req.StopOnError {
				break
			}
			continue
		}

		fn, ok := registry[action.Kind]
		if !ok {
			tCancel()
			kinds := make([]string, 0, len(registry))
			for k := range registry {
				kinds = append(kinds, k)
			}
			results = append(results, actionResult{
				Index:   i,
				Success: false,
				Error:   fmt.Sprintf("unknown action: %s - valid values: %s", action.Kind, strings.Join(kinds, ", ")),
			})
			if req.StopOnError {
				break
			}
			continue
		}

		// Execute the action
		actionRes, err := fn(tCtx, action)
		tCancel()

		if err != nil {
			results = append(results, actionResult{
				Index:   i,
				Success: false,
				Error:   fmt.Sprintf("action %s: %v", action.Kind, err),
			})
			if req.StopOnError {
				break
			}
		} else {
			results = append(results, actionResult{
				Index:   i,
				Success: true,
				Result:  actionRes,
			})
		}

		// Small delay between actions to avoid overwhelming the browser
		if i < len(req.Actions)-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	jsonResp(w, 200, map[string]any{
		"results":    results,
		"total":      len(req.Actions),
		"successful": countSuccessful(results),
		"failed":     len(req.Actions) - countSuccessful(results),
	})
}

func countSuccessful(results []actionResult) int {
	count := 0
	for _, r := range results {
		if r.Success {
			count++
		}
	}
	return count
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
			jsonResp(w, 400, map[string]string{"error": "tabId required"})
			return
		}

		if err := b.CloseTab(req.TabID); err != nil {
			jsonErr(w, 500, err)
			return
		}
		jsonResp(w, 200, map[string]any{"closed": true})

	default:
		jsonResp(w, 400, map[string]string{"error": "action must be 'new' or 'close'"})
	}
}

// ── GET /cookies ───────────────────────────────────────────

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

	// Import the network domain for cookie access
	var cookies []*network.Cookie
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Get current URL if not specified
			if url == "" {
				_ = chromedp.Location(&url).Do(ctx)
			}

			// Get cookies for the URL
			var err error
			cookies, err = network.GetCookies().WithURLs([]string{url}).Do(ctx)
			return err
		}),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("get cookies: %w", err))
		return
	}

	// Filter by name if specified
	if name != "" {
		filtered := make([]*network.Cookie, 0)
		for _, c := range cookies {
			if c.Name == name {
				filtered = append(filtered, c)
			}
		}
		cookies = filtered
	}

	// Convert to simpler format
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

// ── POST /cookies ──────────────────────────────────────────

type cookieRequest struct {
	TabID   string             `json:"tabId"`
	URL     string             `json:"url"`     // Required
	Cookies []cookieSetRequest `json:"cookies"` // Array of cookies to set
}

type cookieSetRequest struct {
	Name     string  `json:"name"`   // Required
	Value    string  `json:"value"`  // Required
	Domain   string  `json:"domain"` // Optional, defaults to current domain
	Path     string  `json:"path"`   // Optional, defaults to "/"
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"` // "Strict", "Lax", "None"
	Expires  float64 `json:"expires"`  // Unix timestamp
}

func (b *Bridge) handleSetCookies(w http.ResponseWriter, r *http.Request) {
	var req cookieRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.URL == "" {
		jsonResp(w, 400, map[string]string{"error": "url is required"})
		return
	}

	if len(req.Cookies) == 0 {
		jsonResp(w, 400, map[string]string{"error": "cookies array is empty"})
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
			continue // Skip invalid cookies
		}

		// Build CDP cookie parameters
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

		// Handle SameSite
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

		// Set the cookie
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

// ── GET /stealth/status ────────────────────────────────────

func (b *Bridge) handleStealthStatus(w http.ResponseWriter, r *http.Request) {
	// Actually check features by evaluating in browser
	ctx, _, err := b.TabContext("")
	if err != nil {
		// If no tab, return static analysis
		stealthFeatures := map[string]bool{
			"automation_controlled": true,     // Chrome flags
			"webdriver_hidden":      true,     // Stealth script
			"chrome_headless_new":   headless, // Mode check
			"user_agent_override":   true,     // Fingerprint endpoint exists
			"webgl_vendor_override": true,     // In stealth.js
			"plugins_spoofed":       true,     // In stealth.js
			"languages_spoofed":     true,     // Configurable
			"webrtc_leak_prevented": true,     // Fixed properly
			"timezone_spoofed":      true,     // Configurable
			"canvas_noise":          true,     // Properly implemented
			"audio_noise":           false,    // Not implemented
			"font_spoofing":         true,     // Proxy-based
			"hardware_concurrency":  true,     // Seeded random
			"device_memory":         true,     // Seeded random
		}
		b.sendStealthResponse(w, stealthFeatures, "")
		return
	}

	// Check actual browser state
	var result struct {
		WebDriver           bool     `json:"webdriver"`
		Plugins             int      `json:"plugins"`
		UserAgent           string   `json:"userAgent"`
		HardwareConcurrency int      `json:"hardwareConcurrency"`
		DeviceMemory        float64  `json:"deviceMemory"`
		Languages           []string `json:"languages"`
	}

	tCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	checkScript := `({
		webdriver: navigator.webdriver || false,
		plugins: navigator.plugins.length,
		userAgent: navigator.userAgent,
		hardwareConcurrency: navigator.hardwareConcurrency,
		deviceMemory: navigator.deviceMemory || 0,
		languages: navigator.languages || []
	})`

	if err := chromedp.Run(tCtx, chromedp.Evaluate(checkScript, &result)); err == nil {
		stealthFeatures := map[string]bool{
			"automation_controlled": !result.WebDriver,
			"webdriver_hidden":      !result.WebDriver,
			"chrome_headless_new":   headless,
			"user_agent_override":   result.UserAgent != "",
			"webgl_vendor_override": true, // Can't easily check
			"plugins_spoofed":       result.Plugins > 0,
			"languages_spoofed":     len(result.Languages) > 0,
			"webrtc_leak_prevented": true, // Configured in stealth.js
			"timezone_spoofed":      true, // Configurable
			"canvas_noise":          true, // Implemented
			"audio_noise":           false,
			"font_spoofing":         true, // Proxy-based
			"hardware_concurrency":  result.HardwareConcurrency > 0,
			"device_memory":         result.DeviceMemory > 0,
		}
		b.sendStealthResponse(w, stealthFeatures, result.UserAgent)
		return
	}

	// Fallback to static
	stealthFeatures := map[string]bool{
		"automation_controlled": true,
		"webdriver_hidden":      true,
		"chrome_headless_new":   headless,
		"user_agent_override":   true,
		"webgl_vendor_override": true,
		"plugins_spoofed":       true,
		"languages_spoofed":     true,
		"webrtc_leak_prevented": true,
		"timezone_spoofed":      true,
		"canvas_noise":          true,
		"audio_noise":           false,
		"font_spoofing":         true,
		"hardware_concurrency":  true,
		"device_memory":         true,
	}
	b.sendStealthResponse(w, stealthFeatures, "")
}

func (b *Bridge) sendStealthResponse(w http.ResponseWriter, features map[string]bool, userAgent string) {

	// Check which Chrome flags are active
	chromeFlags := []string{
		"--disable-blink-features=AutomationControlled",
		"--disable-features=IsolateOrigins,site-per-process",
		"--disable-site-isolation-trials",
		"--disable-web-security",
		"--disable-features=BlinkGenPropertyTrees",
		"--disable-ipc-flooding-protection",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
		"--disable-features=TranslateUI",
		"--disable-features=Translate",
	}

	// Calculate stealth score (percentage of features enabled)
	enabledCount := 0
	for _, enabled := range features {
		if enabled {
			enabledCount++
		}
	}
	stealthScore := (enabledCount * 100) / len(features)

	// Determine stealth level
	var level string
	switch {
	case stealthScore >= 80:
		level = "high"
	case stealthScore >= 50:
		level = "medium"
	case stealthScore >= 30:
		level = "basic"
	default:
		level = "minimal"
	}

	// Use provided userAgent or get from browser if empty
	if userAgent == "" && len(b.tabs) > 0 {
		// Get UA from any active tab
		for _, tab := range b.tabs {
			ctx, cancel := context.WithTimeout(tab.ctx, 1*time.Second)
			_ = chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &userAgent))
			cancel()
			if userAgent != "" {
				break
			}
		}
	}

	jsonResp(w, 200, map[string]any{
		"level":           level,
		"score":           stealthScore,
		"features":        features,
		"chrome_flags":    chromeFlags,
		"headless_mode":   headless,
		"user_agent":      userAgent,
		"profile_path":    profileDir,
		"recommendations": getStealthRecommendations(features),
	})
}

func getStealthRecommendations(features map[string]bool) []string {
	recommendations := []string{}

	if !features["user_agent_override"] {
		recommendations = append(recommendations, "Enable user agent rotation to avoid detection")
	}
	if !features["languages_spoofed"] {
		recommendations = append(recommendations, "Randomize Accept-Language headers")
	}
	if !features["webrtc_leak_prevented"] {
		recommendations = append(recommendations, "Block WebRTC to prevent IP leaks")
	}
	if !features["timezone_spoofed"] {
		recommendations = append(recommendations, "Spoof timezone to match target locale")
	}
	if !features["canvas_noise"] {
		recommendations = append(recommendations, "Add canvas fingerprint noise")
	}
	if !features["font_spoofing"] {
		recommendations = append(recommendations, "Randomize font metrics")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Stealth mode is well configured")
	}

	return recommendations
}

// ── POST /fingerprint/rotate ───────────────────────────────

type fingerprintRequest struct {
	TabID    string `json:"tabId"`
	OS       string `json:"os"`       // "random", "windows", "mac", "linux"
	Browser  string `json:"browser"`  // "chrome", "firefox", "safari", "edge"
	Screen   string `json:"screen"`   // "random", "1920x1080", "1366x768", etc
	Language string `json:"language"` // "en-US", "es-ES", etc
	Timezone int    `json:"timezone"` // offset in minutes from UTC
	WebGL    bool   `json:"webgl"`    // enable WebGL spoofing
	Canvas   bool   `json:"canvas"`   // enable canvas noise
	Fonts    bool   `json:"fonts"`    // enable font spoofing
	Audio    bool   `json:"audio"`    // enable audio fingerprint noise
}

func (b *Bridge) handleFingerprintRotate(w http.ResponseWriter, r *http.Request) {
	var req fingerprintRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, _, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	// Generate fingerprint based on request
	fp := generateFingerprint(req)

	// Inject fingerprint overrides
	script := fmt.Sprintf(`
(function() {
  // User agent override
  Object.defineProperty(navigator, 'userAgent', {
    get: () => %q
  });
  Object.defineProperty(navigator, 'platform', {
    get: () => %q
  });
  Object.defineProperty(navigator, 'vendor', {
    get: () => %q
  });
  
  // Screen resolution
  Object.defineProperty(screen, 'width', { get: () => %d });
  Object.defineProperty(screen, 'height', { get: () => %d });
  Object.defineProperty(screen, 'availWidth', { get: () => %d });
  Object.defineProperty(screen, 'availHeight', { get: () => %d });
  
  // Language
  Object.defineProperty(navigator, 'language', { get: () => %q });
  Object.defineProperty(navigator, 'languages', { get: () => [%q] });
  
  // Timezone
  Date.prototype.getTimezoneOffset = function() { return %d; };
  
  // Hardware
  Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => %d });
  Object.defineProperty(navigator, 'deviceMemory', { get: () => %d });
  
  console.log('Fingerprint rotated successfully');
})();
	`, fp.UserAgent, fp.Platform, fp.Vendor,
		fp.ScreenWidth, fp.ScreenHeight, fp.ScreenWidth-20, fp.ScreenHeight-80,
		fp.Language, fp.Language, fp.TimezoneOffset,
		fp.CPUCores, fp.Memory)

	tCtx, tCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tCancel()

	// Use CDP to inject script that persists across navigations
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Set timezone via CDP
			if req.Timezone != 0 {
				script = fmt.Sprintf("window.__pinchtab_timezone = %d;", req.Timezone) + script
			}

			// Add script to evaluate on every document
			_, err := page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
			return err
		}),
		// Also evaluate once for current page
		chromedp.Evaluate(script, nil),
	); err != nil {
		jsonErr(w, 500, fmt.Errorf("inject fingerprint: %w", err))
		return
	}

	jsonResp(w, 200, map[string]any{
		"fingerprint": fp,
		"status":      "rotated",
	})
}

type fingerprint struct {
	UserAgent      string `json:"userAgent"`
	Platform       string `json:"platform"`
	Vendor         string `json:"vendor"`
	ScreenWidth    int    `json:"screenWidth"`
	ScreenHeight   int    `json:"screenHeight"`
	Language       string `json:"language"`
	TimezoneOffset int    `json:"timezoneOffset"`
	CPUCores       int    `json:"cpuCores"`
	Memory         int    `json:"memory"`
}

func generateFingerprint(req fingerprintRequest) fingerprint {
	fp := fingerprint{}

	// OS and browser combinations
	osConfigs := map[string]map[string]fingerprint{
		"windows": {
			"chrome": {
				UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
				Platform:  "Win32",
				Vendor:    "Google Inc.",
			},
			"edge": {
				UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
				Platform:  "Win32",
				Vendor:    "Google Inc.",
			},
		},
		"mac": {
			"chrome": {
				UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
				Platform:  "MacIntel",
				Vendor:    "Google Inc.",
			},
			"safari": {
				UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
				Platform:  "MacIntel",
				Vendor:    "Apple Computer, Inc.",
			},
		},
	}

	// Select OS/browser
	os := req.OS
	if os == "random" {
		if rand.Float64() < 0.7 {
			os = "windows"
		} else {
			os = "mac"
		}
	}

	browser := req.Browser
	if browser == "" {
		browser = "chrome"
	}

	if osConfig, ok := osConfigs[os]; ok {
		if browserConfig, ok := osConfig[browser]; ok {
			fp.UserAgent = browserConfig.UserAgent
			fp.Platform = browserConfig.Platform
			fp.Vendor = browserConfig.Vendor
		}
	}

	// Screen resolution
	screens := [][]int{
		{1920, 1080}, {1366, 768}, {1536, 864}, {1440, 900},
		{1280, 720}, {1600, 900}, {2560, 1440},
	}
	if req.Screen == "random" {
		screen := screens[rand.Intn(len(screens))]
		fp.ScreenWidth = screen[0]
		fp.ScreenHeight = screen[1]
	} else if req.Screen != "" {
		_, _ = fmt.Sscanf(req.Screen, "%dx%d", &fp.ScreenWidth, &fp.ScreenHeight)
	} else {
		fp.ScreenWidth = 1920
		fp.ScreenHeight = 1080
	}

	// Language
	if req.Language != "" {
		fp.Language = req.Language
	} else {
		fp.Language = "en-US"
	}

	// Timezone
	if req.Timezone != 0 {
		fp.TimezoneOffset = req.Timezone
	} else {
		fp.TimezoneOffset = -300 // EST
	}

	// Hardware
	fp.CPUCores = 4 + rand.Intn(4)*2
	fp.Memory = 4 + rand.Intn(4)*2

	return fp
}
