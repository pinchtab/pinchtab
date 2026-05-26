package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
)

// HandleText extracts readable text from the current tab.
//
// @Endpoint GET /text
func (h *Handlers) HandleText(w http.ResponseWriter, r *http.Request) {
	// Browser resolution: request > session > instance > global default > chrome
	requestBrowser := strings.TrimSpace(r.URL.Query().Get("browser"))
	var sessionBrowser string
	if sess, ok := session.FromRequest(r); ok && sess != nil {
		sessionBrowser = sess.Browser
	}
	var instanceBrowser string
	tabID := r.URL.Query().Get("tabId")
	if tabID != "" && h.Orchestrator != nil {
		if inst, ok := h.Orchestrator.FindInstanceByTab(tabID); ok && inst != nil && inst.Browser != "" {
			instanceBrowser = inst.Browser
		}
	}

	browser := config.ResolveBrowser(requestBrowser, sessionBrowser, instanceBrowser, h.Config.DefaultBrowser, h.Config.BrowsersAvailable)
	if browser != config.BrowserChrome {
		if _, err := config.ParseBrowser(browser, h.Config.BrowsersAvailable); err != nil {
			httpx.Error(w, http.StatusBadRequest, err)
			return
		}
	}

	// --- Ghost-chrome routing fast path ---
	// For ghost-chrome, use the static browser to read text from stored state
	// (set by a prior ghost-chrome /navigate). If the static browser has content,
	// serve it directly. If not available or empty, escalate to Chrome.
	if browser == config.BrowserGhostChrome && h.StaticBrowser != nil {
		h.recordReadRequest(r, "text", tabID)
		result, err := h.StaticBrowser.Text(r.Context(), tabID)
		if err == nil && result.Text != "" {
			// Quality gate: assess ghost content before serving.
			gr := ghostchrome.AssessContent(result.Text)
			if gr.ShouldAccept() {
				// IDPI content scan at handler level.
				scanResult := h.ContentGuard.Scan(result.Text, result.URL)
				if scanResult.Blocked {
					httpx.Error(w, http.StatusForbidden, fmt.Errorf("content blocked: %s", scanResult.BlockReason))
					return
				}
				scanResult.SetHeaders(w)

				route := &browserops.RouteMetadata{
					RequestedBrowser: browser,
					UsedBrowser:      "ghost",
					Attempts:         []browserops.RouteAttempt{{Browser: "ghost", Accepted: true, Reason: gr.FormatReason()}},
				}
				if requestBrowser != "" {
					route.RequestedBrowser = requestBrowser
				}
				h.recordActivity(r, activity.Update{Route: route})

				format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
				if format == "text" || format == "plain" {
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					w.WriteHeader(200)
					_, _ = w.Write([]byte(scanResult.Text))
					return
				}
				httpx.JSON(w, 200, map[string]any{
					"url":   result.URL,
					"title": result.Title,
					"text":  scanResult.Text,
					"route": route,
				})
				return
			}
			// Quality too low — escalate to Chrome (fall through).
		}
		// Ghost text not available or quality insufficient.
		// Fall through to Chrome path.
	}

	handleDecision, err := checkBrowserCanHandle(browser, browsers.RequestIntent{
		Shape: browsers.ShapeRenderedRead,
	})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err)
		return
	}
	if handleDecision.Decision == browsers.DecisionSkip {
		browser = config.BrowserChrome
	}

	// Validate that the resolved browser can be unambiguously mapped to a target.
	textBrowserTarget, err := config.ResolveBrowserToTarget(h.Config, browser)
	if err != nil {
		var ambErr *config.AmbiguousBrowserError
		if errors.As(err, &ambErr) {
			httpx.ErrorCode(w, http.StatusBadRequest, "browser_ambiguous", err.Error(), false, map[string]any{
				"browser": ambErr.Browser,
				"targets": ambErr.Targets,
			})
		} else {
			httpx.Error(w, http.StatusBadRequest, err)
		}
		return
	}

	// Resolve the effective config with target-specific overrides merged in.
	effectiveCfg := h.resolveEffectiveConfig(textBrowserTarget)

	// --- Static browser fast path ---
	h.recordReadRequest(r, "text", tabID)
	if h.useStaticBrowser(browserops.CapText) {
		result, err := h.StaticBrowser.Text(r.Context(), tabID)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("lite text: %w", err))
			return
		}
		// IDPI content scan at handler level.
		scanResult := h.ContentGuard.Scan(result.Text, result.URL)
		if scanResult.Blocked {
			httpx.Error(w, http.StatusForbidden, fmt.Errorf("content blocked: %s", scanResult.BlockReason))
			return
		}
		scanResult.SetHeaders(w)

		result.Route = browserops.SingleBrowserRoute(browser)
		result.Route.Attempts = append(result.Route.Attempts, browserops.RouteAttempt{
			Browser:  browser,
			Accepted: handleDecision.Decision == browsers.DecisionHandle,
			Reason:   handleDecision.Reason,
		})
		if requestBrowser != "" {
			result.Route.RequestedBrowser = requestBrowser
		}
		h.recordActivity(r, activity.Update{Route: result.Route})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(scanResult.Text))
		return
	}

	textRoute := browserops.SingleBrowserRoute("chrome")
	textRoute.Attempts = append(textRoute.Attempts, browserops.RouteAttempt{
		Browser:  browser,
		Accepted: handleDecision.Decision == browsers.DecisionHandle,
		Reason:   handleDecision.Reason,
	})
	if requestBrowser != "" {
		textRoute.RequestedBrowser = requestBrowser
	}
	h.recordActivity(r, activity.Update{Route: textRoute})

	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	mode := r.URL.Query().Get("mode")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	maxChars := -1
	if v := r.URL.Query().Get("maxChars"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxChars = n
		}
	}

	ctx, resolvedTabID, err := h.tabContextWithHeader(w, r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}
	defer h.armAutoCloseIfEnabled(resolvedTabID)

	tCtx, tCancel := context.WithTimeout(ctx, effectiveCfg.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	// Resolve the target frame. Explicit ?frameId= wins; otherwise fall back
	// to the currently-scoped frame on this tab (as set by /frame). Empty
	// means "top-level document" and preserves the prior behaviour.
	targetFrameID := r.URL.Query().Get("frameId")
	if targetFrameID == "" {
		if scope, ok := h.currentFrameScope(resolvedTabID); ok {
			targetFrameID = scope.FrameID
		}
	}

	// Auto-wait: if the document is still loading, wait for readyState to
	// reach at least "interactive" before extracting text. Prevents empty or
	// partial results when text is called before the page finishes loading.
	waitForReadyState(tCtx)

	// Handle element selector - extract text from specific element instead of full page
	selectorParam := r.URL.Query().Get("selector")
	refParam := r.URL.Query().Get("ref")
	if selectorParam != "" || refParam != "" {
		text, err := h.extractElementText(tCtx, resolvedTabID, selectorParam, refParam)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("element text extract: %w", err))
			return
		}
		var url, title string
		_ = chromedp.Run(tCtx,
			chromedp.Location(&url),
			chromedp.Title(&title),
		)
		h.recordResolvedURL(r, url)
		httpx.JSON(w, 200, map[string]any{
			"url":   url,
			"title": title,
			"text":  text,
		})
		return
	}

	script := `document.body.innerText`
	if mode != "raw" {
		script = assets.ReadabilityJS
	}

	var text string
	if targetFrameID == "" {
		// Cross-frame path: collect text from all reachable frames (same
		// as snap, which auto-flattens same-origin iframes). Cross-origin
		// frames are silently skipped because createIsolatedWorld fails.
		text = h.extractTextAllFrames(tCtx, script)
	} else {
		// Frame-scoped path — evaluate in the frame's isolated world so the
		// expression sees the iframe's `document`, not the parent's.
		var err error
		text, err = h.evalTextInFrame(tCtx, script, targetFrameID)
		if err != nil {
			httpx.Error(w, 500, err)
			return
		}
	}

	truncated := false
	if maxChars > -1 && len(text) > maxChars {
		text = text[:maxChars]
		truncated = true
	}

	var url, title string
	_ = chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)
	h.recordResolvedURL(r, url)

	// IDPI: scan extracted text for injection patterns and optionally wrap.
	result := h.ContentGuard.Scan(text, url)
	if result.Blocked {
		httpx.Error(w, http.StatusForbidden, fmt.Errorf("content blocked by IDPI scanner: %s", result.BlockReason))
		return
	}
	result.SetHeaders(w)
	text = result.Text

	if format == "text" || format == "plain" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(text))
		return
	}

	resp := map[string]any{
		"url":       url,
		"title":     title,
		"text":      text,
		"truncated": truncated,
		"route":     textRoute,
	}
	if result.Warning != "" {
		resp["idpiWarning"] = result.Warning
	}
	httpx.JSON(w, 200, resp)
}

// HandleTabText extracts text for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/text
func (h *Handlers) HandleTabText(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	q := r.URL.Query()
	q.Set("tabId", tabID)

	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u

	h.HandleText(w, req)
}

// extractTextAllFrames evaluates the text script in every reachable frame and
// concatenates the results. Cross-origin frames are silently skipped (the
// isolated-world creation fails, just like snap skips inaccessible frames).
func (h *Handlers) extractTextAllFrames(ctx context.Context, script string) string {
	frameTree, err := observe.FetchFrameTree(ctx)
	if err != nil {
		// Fallback: top frame only.
		var text string
		_ = chromedp.Run(ctx, chromedp.Evaluate(script, &text))
		return text
	}

	ids := observe.FrameIDs(frameTree)
	if len(ids) == 0 {
		var text string
		_ = chromedp.Run(ctx, chromedp.Evaluate(script, &text))
		return text
	}

	var parts []string
	for _, id := range ids {
		t, err := h.evalTextInFrame(ctx, script, id)
		if err != nil || strings.TrimSpace(t) == "" {
			continue
		}
		parts = append(parts, t)
	}
	if len(parts) == 0 {
		var text string
		_ = chromedp.Run(ctx, chromedp.Evaluate(script, &text))
		return text
	}
	return strings.Join(parts, "\n\n")
}

// evalTextInFrame evaluates a text-extraction script in a specific frame's
// isolated world and returns the result string.
func (h *Handlers) evalTextInFrame(ctx context.Context, script, frameID string) (string, error) {
	execID, err := bridge.FrameExecutionContextID(ctx, frameID)
	if err != nil {
		return "", fmt.Errorf("resolve frame context: %w", err)
	}
	if execID == 0 {
		// Top frame — use the simpler chromedp path.
		var text string
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &text)); err != nil {
			return "", fmt.Errorf("text extract: %w", err)
		}
		return text, nil
	}
	var raw json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    script,
			"returnByValue": true,
			"contextId":     execID,
		}, &raw)
	}))
	if err != nil {
		return "", fmt.Errorf("text extract (frame %s): %w", frameID, err)
	}
	var er struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(raw, &er); err != nil {
		return "", fmt.Errorf("text extract parse: %w", err)
	}
	if er.ExceptionDetails != nil && er.ExceptionDetails.Text != "" {
		return "", fmt.Errorf("text extract (frame %s): %s", frameID, er.ExceptionDetails.Text)
	}
	return er.Result.Value, nil
}

// extractElementText extracts innerText from a specific element by selector or ref.
func (h *Handlers) extractElementText(ctx context.Context, tabID, selector, ref string) (string, error) {
	var text string

	if ref != "" {
		cache := h.Bridge.GetRefCache(tabID)
		if cache == nil {
			return "", fmt.Errorf("ref not found: %s (no snapshot cache)", ref)
		}
		target, ok := cache.Lookup(ref)
		if !ok {
			return "", fmt.Errorf("ref not found: %s", ref)
		}
		nodeID := target.BackendNodeID

		err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			var objRes struct {
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			}
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
				"backendNodeId": nodeID,
			}, &objRes); err != nil {
				return err
			}
			if objRes.Object.ObjectID == "" {
				return fmt.Errorf("could not resolve node")
			}
			var callRes struct {
				Result struct {
					Value string `json:"value"`
				} `json:"result"`
			}
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
				"functionDeclaration": `function() { return this.innerText || this.textContent || ''; }`,
				"objectId":            objRes.Object.ObjectID,
				"returnByValue":       true,
			}, &callRes); err != nil {
				return err
			}
			text = callRes.Result.Value
			return nil
		}))
		if err != nil {
			return "", err
		}
		return text, nil
	}

	var script string
	switch {
	case strings.HasPrefix(selector, "xpath:"):
		xpath := selector[len("xpath:"):]
		script = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);var n=r.singleNodeValue;return n?(n.innerText||n.textContent||''):null})()`, xpath)
	case strings.HasPrefix(selector, "//") || strings.HasPrefix(selector, "(//"):
		script = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);var n=r.singleNodeValue;return n?(n.innerText||n.textContent||''):null})()`, selector)
	case strings.HasPrefix(selector, "text:"):
		textVal := selector[len("text:"):]
		script = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%q))return w.currentNode.parentElement.innerText||w.currentNode.parentElement.textContent||''}return null})()`, textVal)
	case strings.HasPrefix(selector, "css:"):
		css := selector[len("css:"):]
		script = fmt.Sprintf(`(function(){var n=document.querySelector(%q);return n?(n.innerText||n.textContent||''):null})()`, css)
	default:
		script = fmt.Sprintf(`(function(){var n=document.querySelector(%q);return n?(n.innerText||n.textContent||''):null})()`, selector)
	}

	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &text)); err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("no element matches selector: %s", selector)
	}
	return text, nil
}

func waitForReadyState(ctx context.Context) {
	var state string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &state)); err != nil {
		return
	}
	if state != "loading" {
		return
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			return
		case <-time.After(100 * time.Millisecond):
			if err := chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &state)); err != nil {
				return
			}
			if state != "loading" {
				return
			}
		}
	}
}
