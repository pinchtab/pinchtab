package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const (
	maxWaitTimeout   = 30_000 // 30s max
	defaultTimeout   = 10_000 // 10s default
	pollInterval     = 250 * time.Millisecond
	maxFixedWaitMS   = 30_000
	defaultIdleForMS = 500
	maxIdleForMS     = 10_000
	networkIdlePoll  = 100 * time.Millisecond
)

// waitRequest is the JSON body for POST /wait and POST /tabs/{id}/wait.
type waitRequest struct {
	TabID    string `json:"tabId,omitempty"`
	Selector string `json:"selector,omitempty"` // CSS/XPath/text selector
	State    string `json:"state,omitempty"`    // "visible" (default) or "hidden"
	Text     string `json:"text,omitempty"`     // wait for text on page
	NotText  string `json:"notText,omitempty"`  // wait for text to disappear from page
	URL      string `json:"url,omitempty"`      // wait for URL glob match
	Load     string `json:"load,omitempty"`     // "ready-state" | "content-loaded" | "network-idle" (alias: networkidle)
	Fn       string `json:"fn,omitempty"`       // JS expression to poll for truthy
	Ms       *int   `json:"ms,omitempty"`       // fixed duration wait
	Timeout  *int   `json:"timeout,omitempty"`  // timeout in ms
	IdleFor  *int   `json:"idleFor,omitempty"`  // network-idle quiet period in ms (default 500, max 10000)
}

// waitResponse is the JSON response for wait endpoints.
type waitResponse struct {
	Waited  bool   `json:"waited"`
	Elapsed int64  `json:"elapsed"`
	Match   string `json:"match,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (wr *waitRequest) mode() string {
	switch {
	case wr.Ms != nil:
		return "ms"
	case wr.Selector != "":
		return "selector"
	case wr.Text != "":
		return "text"
	case wr.NotText != "":
		return "notText"
	case wr.URL != "":
		return "url"
	case wr.Load != "":
		return "load"
	case wr.Fn != "":
		return "fn"
	default:
		return ""
	}
}

func (wr *waitRequest) resolvedTimeout() time.Duration {
	ms := defaultTimeout
	if wr.Timeout != nil {
		ms = *wr.Timeout
	}
	if ms < 100 {
		ms = 100
	}
	if ms > maxWaitTimeout {
		ms = maxWaitTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func (wr *waitRequest) resolvedIdleFor() time.Duration {
	ms := defaultIdleForMS
	if wr.IdleFor != nil {
		ms = *wr.IdleFor
	}
	if ms < 0 {
		ms = 0
	}
	if ms > maxIdleForMS {
		ms = maxIdleForMS
	}
	return time.Duration(ms) * time.Millisecond
}

// canonicalLoadState maps the user-supplied --load value to a canonical
// state name and returns false if the value is not recognised. Accepts
// the legacy "networkidle" spelling as an alias for "network-idle".
func canonicalLoadState(s string) (string, bool) {
	switch s {
	case "ready-state", "content-loaded", "network-idle":
		return s, true
	case "networkidle":
		return "network-idle", true
	default:
		return "", false
	}
}

// HandleWait handles POST /wait.
//
// @Endpoint POST /wait
func (h *Handlers) HandleWait(w http.ResponseWriter, r *http.Request) {
	var req waitRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	h.handleWaitCore(w, r, req)
}

// HandleTabWait handles POST /tabs/{id}/wait.
//
// @Endpoint POST /tabs/{id}/wait
func (h *Handlers) HandleTabWait(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			httpx.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	body["tabId"] = tabID

	payload, err := json.Marshal(body)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	cloned := r.Clone(r.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(payload))
	cloned.ContentLength = int64(len(payload))
	cloned.Header = r.Header.Clone()
	cloned.Header.Set("Content-Type", "application/json")
	h.HandleWait(w, cloned)
}

func (h *Handlers) handleWaitCore(w http.ResponseWriter, r *http.Request, req waitRequest) {
	start := time.Now()

	mode := req.mode()
	if mode == "" {
		httpx.Error(w, 400, fmt.Errorf("one of selector, text, url, load, fn, or ms is required"))
		return
	}

	h.recordActivity(r, activity.Update{Action: "wait." + mode, TabID: req.TabID})
	if mode == "fn" && !h.evaluateEnabled() {
		httpx.ErrorCode(w, 403, "evaluate_disabled", httpx.DisabledEndpointMessage("evaluate", "security.allowEvaluate"), false, map[string]any{
			"setting": "security.allowEvaluate",
		})
		return
	}

	// Fixed duration wait doesn't need a browser tab.
	if mode == "ms" {
		ms := *req.Ms
		if ms < 0 {
			ms = 0
		}
		if ms > maxFixedWaitMS {
			ms = maxFixedWaitMS
		}
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
			httpx.JSON(w, 200, waitResponse{
				Waited:  true,
				Elapsed: time.Since(start).Milliseconds(),
				Match:   fmt.Sprintf("%dms", ms),
			})
		case <-r.Context().Done():
			httpx.JSON(w, 200, waitResponse{
				Waited:  false,
				Elapsed: time.Since(start).Milliseconds(),
				Error:   "cancelled",
			})
		}
		return
	}

	// All other modes need a browser tab.
	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	timeout := req.resolvedTimeout()
	tCtx, tCancel := context.WithTimeout(ctx, timeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	var js string
	var matchLabel string

	switch mode {
	case "selector":
		js, matchLabel = buildSelectorJS(req.Selector, req.State)
	case "text":
		js = fmt.Sprintf(`document.body && document.body.innerText.includes(%s)`, jsonStr(req.Text))
		matchLabel = req.Text
	case "notText":
		js = fmt.Sprintf(`!document.body || !document.body.innerText.includes(%s)`, jsonStr(req.NotText))
		matchLabel = "!" + req.NotText
	case "url":
		js = buildURLMatchJS(req.URL)
		matchLabel = req.URL
	case "load":
		canonical, ok := canonicalLoadState(req.Load)
		if !ok {
			httpx.Error(w, 400, fmt.Errorf("unsupported load state: %s (supported: ready-state, content-loaded, network-idle)", req.Load))
			return
		}
		matchLabel = canonical
		switch canonical {
		case "ready-state":
			js = `document.readyState === 'complete'`
		case "content-loaded":
			js = `document.readyState === 'interactive' || document.readyState === 'complete'`
		case "network-idle":
			h.handleNetworkIdleWait(w, tCtx, start, resolvedTabID, req.resolvedIdleFor())
			return
		}
	case "fn":
		js = fmt.Sprintf(`!!(function(){try{return %s}catch(e){return false}})()`, req.Fn)
		matchLabel = "fn"
	}

	// Poll loop
	for {
		var result bool
		evalErr := chromedp.Run(tCtx, chromedp.Evaluate(js, &result))
		if evalErr == nil && result {
			httpx.JSON(w, 200, waitResponse{
				Waited:  true,
				Elapsed: time.Since(start).Milliseconds(),
				Match:   matchLabel,
			})
			return
		}

		select {
		case <-tCtx.Done():
			elapsed := time.Since(start).Milliseconds()
			httpx.JSON(w, 200, waitResponse{
				Waited:  false,
				Elapsed: elapsed,
				Error:   fmt.Sprintf("timeout after %dms waiting for %s", elapsed, mode),
			})
			return
		case <-time.After(pollInterval):
			// continue polling
		}
	}
}

// handleNetworkIdleWait polls the per-tab network monitor until in-flight
// requests have stayed at zero for idleFor, or the context is cancelled.
// Falls back to a readyState=='complete' check if the monitor is unavailable
// (e.g. tab created without network capture).
func (h *Handlers) handleNetworkIdleWait(w http.ResponseWriter, ctx context.Context, start time.Time, tabID string, idleFor time.Duration) {
	mon := h.Bridge.NetworkMonitor()
	var buf *bridge.NetworkBuffer
	if mon != nil {
		buf = mon.GetBuffer(tabID)
	}
	if buf == nil {
		httpx.JSON(w, 200, waitResponse{
			Waited:  false,
			Elapsed: time.Since(start).Milliseconds(),
			Error:   "network monitor unavailable for tab",
		})
		return
	}

	ticker := time.NewTicker(networkIdlePoll)
	defer ticker.Stop()

	for {
		count, lastChange := buf.InflightStatus()
		if count == 0 && time.Since(lastChange) >= idleFor {
			httpx.JSON(w, 200, waitResponse{
				Waited:  true,
				Elapsed: time.Since(start).Milliseconds(),
				Match:   "network-idle",
			})
			return
		}

		select {
		case <-ctx.Done():
			elapsed := time.Since(start).Milliseconds()
			httpx.JSON(w, 200, waitResponse{
				Waited:  false,
				Elapsed: elapsed,
				Error:   fmt.Sprintf("timeout after %dms waiting for network-idle (inflight=%d)", elapsed, count),
			})
			return
		case <-ticker.C:
		}
	}
}

// buildSelectorJS builds a JS expression for selector wait.
// Supports css:, xpath:, text: prefixes and bare CSS selectors.
func buildSelectorJS(sel, state string) (string, string) {
	hidden := state == "hidden"

	var js string
	switch {
	case hasPrefix(sel, "xpath:"):
		xpath := sel[len("xpath:"):]
		if hidden {
			js = fmt.Sprintf(`(function(){try{var r=document.evaluate(%s,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue===null}catch(e){return true}})()`, jsonStr(xpath))
		} else {
			js = fmt.Sprintf(`(function(){try{var r=document.evaluate(%s,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue!==null}catch(e){return false}})()`, jsonStr(xpath))
		}
	case hasPrefix(sel, "//") || hasPrefix(sel, "(//"):
		if hidden {
			js = fmt.Sprintf(`(function(){try{var r=document.evaluate(%s,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue===null}catch(e){return true}})()`, jsonStr(sel))
		} else {
			js = fmt.Sprintf(`(function(){try{var r=document.evaluate(%s,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue!==null}catch(e){return false}})()`, jsonStr(sel))
		}
	case hasPrefix(sel, "text:"):
		text := sel[len("text:"):]
		if hidden {
			js = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%s))return false}return true})()`, jsonStr(text))
		} else {
			js = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%s))return true}return false})()`, jsonStr(text))
		}
	default:
		css := sel
		if hasPrefix(sel, "css:") {
			css = sel[len("css:"):]
		}
		if hidden {
			js = fmt.Sprintf(`document.querySelector(%s) === null`, jsonStr(css))
		} else {
			js = fmt.Sprintf(`document.querySelector(%s) !== null`, jsonStr(css))
		}
	}

	return js, sel
}

// buildURLMatchJS builds a JS expression that checks if the current URL matches a glob pattern.
func buildURLMatchJS(pattern string) string {
	// Convert glob to regex: ** → .*, * → [^/]*, ? → .
	// For simplicity, we use a JS function that does basic glob matching.
	return fmt.Sprintf(`(function(){
		var p = %s;
		var u = window.location.href;
		// Convert glob to regex
		var re = p.replace(/[.+^${}()|[\\]\\\\]/g, '\\\\$&')
		           .replace(/\\*\\*/g, '<<<DOUBLESTAR>>>')
		           .replace(/\\*/g, '[^/]*')
		           .replace(/<<<DOUBLESTAR>>>/g, '.*')
		           .replace(/\\?/g, '.');
		return new RegExp(re).test(u);
	})()`, jsonStr(pattern))
}

// jsonStr returns a JSON-encoded string (with quotes).
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
