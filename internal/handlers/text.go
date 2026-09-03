package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/selector"
	"unicode/utf8"
)

// @Endpoint GET /text
func (h *Handlers) HandleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	effectiveCfg, textRoute, ok := h.resolveReadRouting(w, r, tabID, "text", browsers.ShapeRenderedRead)
	if !ok {
		return
	}

	if !h.ensureBrowserOrRespond(w, effectiveCfg) {
		return
	}

	mode, modeErr := resolveTextMode(r.URL.Query().Get("mode"))
	if modeErr != nil {
		httpx.Error(w, 400, modeErr)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	maxChars := -1
	if v := r.URL.Query().Get("maxChars"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxChars = n
		}
	}

	resolvedTabID, tCtx, cancel, ok := h.resolveReadContext(w, r, tabID, effectiveCfg.ActionTimeout)
	if !ok {
		return
	}
	defer h.armAutoCloseIfEnabled(resolvedTabID)
	defer cancel()

	targetFrameID := h.resolveTargetFrameID(r, resolvedTabID)

	// Auto-wait: if the document is still loading, wait for readyState to
	// reach at least "interactive" before extracting text. Prevents empty or
	// partial results when text is called before the page finishes loading.
	h.waitForReadyState(tCtx)
	selectorParam := r.URL.Query().Get("selector")
	refParam := r.URL.Query().Get("ref")
	ghostRoute := textRoute != nil && textRoute.UsedBrowser == config.BrowserGhostChrome
	for attempt := 0; attempt < 2; attempt++ {
		var modalNodeID int64
		var modalOpen bool
		var err error
		if !ghostRoute {
			modalNodeID, modalOpen, err = bridge.TopmostModalNodeID(tCtx, targetFrameID)
			if err != nil {
				httpx.Error(w, selectorFailureStatus(err), err)
				return
			}
		}

		var text string
		var extraction textExtraction
		if selectorParam != "" || refParam != "" {
			text, err = h.extractElementText(tCtx, resolvedTabID, selectorParam, refParam, modalNodeID)
		} else if modalOpen {
			err = h.Bridge.CallFunctionOnNode(tCtx, modalNodeID,
				`function() { return this.innerText || this.textContent || ''; }`, nil, &text)
			extraction = textExtraction{Text: text, Mode: extractionRaw, RawLength: utf8.RuneCountInString(text), RawKnown: true}
		} else {
			extraction, err = h.extractDocumentText(tCtx, mode, targetFrameID)
		}

		stable := true
		if !ghostRoute {
			afterNodeID, afterOpen, scopeErr := bridge.TopmostModalNodeID(tCtx, targetFrameID)
			if scopeErr != nil {
				httpx.Error(w, selectorFailureStatus(scopeErr), fmt.Errorf("recheck topmost dialog: %w", scopeErr))
				return
			}
			stable = modalNodeID == afterNodeID && modalOpen == afterOpen
		}
		if !stable {
			continue
		}
		if err != nil {
			status := http.StatusInternalServerError
			if selectorParam != "" || refParam != "" {
				status = selectorFailureStatus(err)
				err = fmt.Errorf("element text extract: %w", err)
			}
			httpx.Error(w, status, err)
			return
		}
		scopeInfo := h.frameDisclosureFor(tCtx, resolvedTabID, targetFrameID)
		if selectorParam != "" || refParam != "" {
			h.writeElementTextResponse(w, r, tCtx, text, scopeInfo)
		} else {
			h.writeTextResponse(w, r, tCtx, extraction, maxChars, format, textRoute, scopeInfo)
		}
		return
	}
	httpx.Error(w, http.StatusConflict, fmt.Errorf("topmost dialog changed twice during text extraction; retry after the page settles"))
}

// writeElementTextResponse writes an already scope-validated element read.
func (h *Handlers) writeElementTextResponse(w http.ResponseWriter, r *http.Request, tCtx context.Context, text string, scope *frameDisclosure) {
	url, _ := h.Bridge.CurrentURL(tCtx)
	title, _ := h.Bridge.CurrentTitle(tCtx)
	h.recordResolvedURL(r, url)
	httpx.JSON(w, 200, scope.attach(map[string]any{
		"url":   url,
		"title": title,
		"text":  text,
	}))
}

const rawTextScript = `document.body.innerText`

// textModes are the values ?mode= accepts, each mapped to the extraction it selects.
// "full" is an ALIAS of raw because that is what the CLI's --full already sends and
// what the word plainly reads as — the whole unfiltered page. It used to fall through
// to readability, so a caller who read the CLI help and wrote ?mode=full against the
// API got the filtered output while believing they had asked for innerText.
var textModes = map[string]string{
	"":     "",
	"raw":  "raw",
	"full": "raw",
}

// resolveTextMode maps a requested mode onto the extraction that runs, and REFUSES an
// unimplemented one. Silently extracting readability for a mode nobody implemented hands
// the caller something other than what they asked for with no signal, which is the same
// defect class as losing a field boundary.
func resolveTextMode(requested string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(requested))
	mode, ok := textModes[normalized]
	if !ok {
		return "", fmt.Errorf("unknown text mode %q; accepted values are %s (omit mode for the default Readability extraction)", requested, strings.Join(namedTextModes(), ", "))
	}
	return mode, nil
}

// namedTextModes lists the modes a caller can spell, sorted so the refusal reads the
// same on every run rather than however the map happened to iterate.
func namedTextModes() []string {
	named := make([]string, 0, len(textModes))
	for mode := range textModes {
		if mode != "" {
			named = append(named, mode)
		}
	}
	sort.Strings(named)
	return named
}

// Extraction modes echoed to the caller: which extractor produced the text.
const (
	extractionReadability         = "readability"
	extractionRaw                 = "raw"
	extractionReadabilityFallback = "readability_fallback"
)

// Readability is an article heuristic; on a layout it does not recognise as an
// article (landing page, dashboard, docs index) it returns the single block it
// scored highest. Measured against document.body.innerText, a collapse retains
// ~3% of the page while healthy article extraction retains ~91%, so any ratio in
// the low tens of percent separates the two with wide margin on both sides.
const readabilityCoverageRatio = 0.25

// Under this many raw characters the ratio is noise — a short page legitimately
// extracts to a couple of lines — so no fallback fires below it.
const readabilityCoverageFloorChars = 400

// headerTextExtraction reports the extraction mode on the format=text path,
// which returns a bare body with nowhere to carry the signal.
const headerTextExtraction = "X-PT-Text-Extraction"

// textExtraction is the document text plus the story of how it was produced:
// the mode that actually ran and the length of the raw document it was measured
// against (RawKnown is false when the baseline extraction itself failed).
type textExtraction struct {
	Text      string
	Mode      string
	RawLength int
	RawKnown  bool
}

// extractDocumentText reads the document's text (readability unless mode=="raw")
// across all reachable frames, or scoped to targetFrameID when set. Readability
// output that covers too little of the raw document is discarded in favour of
// the raw text, reported as extractionReadabilityFallback.
func (h *Handlers) extractDocumentText(tCtx context.Context, mode, targetFrameID string) (textExtraction, error) {
	if mode == "raw" {
		text, err := h.extractText(tCtx, rawTextScript, targetFrameID)
		if err != nil {
			return textExtraction{}, err
		}
		return textExtraction{Text: text, Mode: extractionRaw, RawLength: utf8.RuneCountInString(text), RawKnown: true}, nil
	}

	text, err := h.extractText(tCtx, assets.ReadabilityJS, targetFrameID)
	if err != nil {
		return textExtraction{}, err
	}

	raw, rawErr := h.extractText(tCtx, rawTextScript, targetFrameID)
	if rawErr != nil {
		return textExtraction{Text: text, Mode: extractionReadability}, nil
	}
	extractedLen, rawLen := utf8.RuneCountInString(text), utf8.RuneCountInString(raw)
	if readabilityCollapsed(extractedLen, rawLen) {
		return textExtraction{Text: raw, Mode: extractionReadabilityFallback, RawLength: rawLen, RawKnown: true}, nil
	}
	return textExtraction{Text: text, Mode: extractionReadability, RawLength: rawLen, RawKnown: true}, nil
}

func readabilityCollapsed(extractedLen, rawLen int) bool {
	return rawLen >= readabilityCoverageFloorChars && float64(extractedLen) < float64(rawLen)*readabilityCoverageRatio
}

// extractText runs one extraction script through the traversal the request
// selected: every reachable frame (joined), or the scoped frame's isolated world
// so the expression sees the iframe's `document`, not the parent's. Both sides of
// the coverage ratio go through here, so they are always built identically.
func (h *Handlers) extractText(tCtx context.Context, script, targetFrameID string) (string, error) {
	if targetFrameID == "" {
		return h.extractTextAllFrames(tCtx, script), nil
	}
	return h.evalTextInFrame(tCtx, script, targetFrameID)
}

// truncateChars cuts s to at most limit characters, reporting whether it cut.
// Slicing by byte splits multi-byte characters and the orphaned bytes reach the
// client as U+FFFD.
func truncateChars(s string, limit int) (string, bool) {
	if limit < 0 || utf8.RuneCountInString(s) <= limit {
		return s, false
	}
	count := 0
	for offset := range s {
		if count == limit {
			return s[:offset], true
		}
		count++
	}
	return s, false
}

// writeTextResponse truncates, IDPI-scans, and writes the document text as
// plain text (format text/plain) or the JSON envelope.
func (h *Handlers) writeTextResponse(w http.ResponseWriter, r *http.Request, tCtx context.Context, extraction textExtraction, maxChars int, format string, route *browserops.RouteMetadata, scope *frameDisclosure) {
	text := extraction.Text
	truncated := false
	if maxChars > -1 {
		text, truncated = truncateChars(text, maxChars)
	}

	url, _ := h.Bridge.CurrentURL(tCtx)
	title, _ := h.Bridge.CurrentTitle(tCtx)
	h.recordResolvedURL(r, url)

	// IDPI: scan extracted text for injection patterns and optionally wrap.
	result := h.ContentGuard.Scan(text, url)
	if result.Blocked {
		httpx.Error(w, http.StatusForbidden, fmt.Errorf("content blocked by IDPI scanner: %s%s", result.BlockReason, idpiScannerHint()))
		return
	}
	result.SetHeaders(w)
	text = result.Text
	w.Header().Set(headerTextExtraction, extraction.Mode)

	if format == "text" || format == "plain" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(text))
		return
	}

	resp := scope.attach(map[string]any{
		"url":        url,
		"title":      title,
		"text":       text,
		"truncated":  truncated,
		"route":      route,
		"extraction": extraction.Mode,
		"textLength": utf8.RuneCountInString(text),
	})
	if extraction.RawKnown {
		resp["rawLength"] = extraction.RawLength
	}
	if result.Warning != "" {
		resp["idpiWarning"] = result.Warning
	}
	httpx.JSON(w, 200, resp)
}

// @Endpoint GET /tabs/{id}/text
func (h *Handlers) HandleTabText(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleText)
}

// extractTextAllFrames evaluates the text script in every reachable frame and
// concatenates the results. Cross-origin frames are silently skipped (the
// isolated-world creation fails, just like snap skips inaccessible frames).
func (h *Handlers) extractTextAllFrames(ctx context.Context, script string) string {
	frameTree, err := observe.FetchFrameTree(ctx)
	if err != nil {
		// Fallback: top frame only.
		var text string
		_ = h.Bridge.Evaluate(ctx, script, &text, bridge.EvalOpts{})
		return text
	}

	ids := observe.FrameIDs(frameTree)
	if len(ids) == 0 {
		var text string
		_ = h.Bridge.Evaluate(ctx, script, &text, bridge.EvalOpts{})
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
		_ = h.Bridge.Evaluate(ctx, script, &text, bridge.EvalOpts{})
		return text
	}
	return strings.Join(parts, "\n\n")
}

// evalTextInFrame evaluates a text-extraction script in a specific frame's
// isolated world and returns the result string.
func (h *Handlers) evalTextInFrame(ctx context.Context, script, frameID string) (string, error) {
	var text string
	if err := h.Bridge.EvaluateInFrame(ctx, frameID, script, &text, bridge.EvalOpts{}); err != nil {
		if frameID != "" {
			return "", fmt.Errorf("text extract (frame %s): %w", frameID, err)
		}
		return "", fmt.Errorf("text extract: %w", err)
	}
	return text, nil
}

func (h *Handlers) extractElementText(ctx context.Context, tabID, selectorValue, ref string, scopeBackendNodeID int64) (string, error) {
	var text string
	if scopeBackendNodeID != 0 {
		var sel selector.Selector
		if ref != "" {
			sel = selector.Parse("ref:" + ref)
		} else {
			sel = selector.Parse(selectorValue)
		}
		nodeID, err := bridge.ResolveUnifiedSelectorWithinNode(ctx, sel, h.Bridge.GetRefCache(tabID), scopeBackendNodeID)
		if err != nil {
			return "", err
		}
		if err := h.Bridge.CallFunctionOnNode(ctx, nodeID,
			`function() { return this.innerText || this.textContent || ''; }`, nil, &text); err != nil {
			return "", err
		}
		return text, nil
	}

	if ref != "" {
		cache := h.Bridge.GetRefCache(tabID)
		if cache == nil {
			return "", fmt.Errorf("ref not found: %s (no snapshot cache): %w", ref, bridge.ErrSelectorNoMatch)
		}
		target, ok := cache.Lookup(ref)
		if !ok {
			return "", fmt.Errorf("ref not found: %s: %w", ref, bridge.ErrSelectorNoMatch)
		}
		nodeID := target.BackendNodeID

		err := h.Bridge.CallFunctionOnNode(ctx, nodeID,
			`function() { return this.innerText || this.textContent || ''; }`,
			nil, &text)
		if err != nil {
			return "", err
		}
		return text, nil
	}

	var script string
	switch {
	case strings.HasPrefix(selectorValue, "xpath:"):
		xpath := selectorValue[len("xpath:"):]
		script = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);var n=r.singleNodeValue;return n?(n.innerText||n.textContent||''):null})()`, xpath)
	case strings.HasPrefix(selectorValue, "//") || strings.HasPrefix(selectorValue, "(//"):
		script = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);var n=r.singleNodeValue;return n?(n.innerText||n.textContent||''):null})()`, selectorValue)
	case strings.HasPrefix(selectorValue, "text:"):
		textVal := selectorValue[len("text:"):]
		script = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%q))return w.currentNode.parentElement.innerText||w.currentNode.parentElement.textContent||''}return null})()`, textVal)
	case strings.HasPrefix(selectorValue, "css:"):
		css := selectorValue[len("css:"):]
		script = fmt.Sprintf(`(function(){var n=document.querySelector(%q);return n?(n.innerText||n.textContent||''):null})()`, css)
	default:
		script = fmt.Sprintf(`(function(){var n=document.querySelector(%q);return n?(n.innerText||n.textContent||''):null})()`, selectorValue)
	}

	if err := h.Bridge.Evaluate(ctx, script, &text, bridge.EvalOpts{}); err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("no element matches selector %q: %w", selectorValue, bridge.ErrSelectorNoMatch)
	}
	return text, nil
}

func (h *Handlers) waitForReadyState(ctx context.Context) {
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// An eval error or a non-loading state both stop the wait, matching the
	// original loop which returned (silently) on either condition.
	_ = pollUntil(wctx, 100*time.Millisecond, func() (bool, error) {
		var state string
		if err := h.Bridge.Evaluate(wctx, `document.readyState`, &state, bridge.EvalOpts{}); err != nil {
			return true, nil
		}
		return state != "loading", nil
	})
}
