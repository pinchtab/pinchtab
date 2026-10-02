package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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
	"github.com/pinchtab/pinchtab/internal/scrape"
	"github.com/pinchtab/pinchtab/internal/selector"
	"unicode/utf8"
)

// @Endpoint GET /text
// @Param mode string query Extraction mode: "" default Readability, "raw"/"full" whole-page innerText, "markdown" seaportal Markdown of the rendered page (falls back to raw text as "markdown_fallback" when the converter yields nothing); any other value is a 400 (optional)
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
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			httpx.ErrorCode(w, http.StatusBadRequest, "invalid_max_chars", fmt.Sprintf("maxChars must be a non-negative integer, got %q", v), false, nil)
			return
		}
		maxChars = n
	}

	resolvedTabID, tCtx, cancel, ok := h.resolveReadContext(w, r, tabID, effectiveCfg.ActionTimeout)
	if !ok {
		return
	}
	defer h.armIdleLifecycle(resolvedTabID)
	defer cancel()

	targetFrameID := h.resolveTargetFrameID(r, resolvedTabID)

	// Auto-wait: if the document is still loading, wait for readyState to
	// reach at least "interactive" before extracting text. Prevents empty or
	// partial results when text is called before the page finishes loading.
	h.waitForReadyState(tCtx)
	selectorParam := r.URL.Query().Get("selector")
	refParam := r.URL.Query().Get("ref")
	ghostRoute := textRoute != nil && textRoute.UsedBrowser == config.BrowserGhostChrome
	vocabBefore := h.tabVocab(resolvedTabID)
	elementRead := selectorParam != "" || refParam != ""
	var text string
	var extraction textExtraction
	workErr, scopeErr := withStableModalScope(tCtx, targetFrameID, ghostRoute, func(modalNodeID int64, modalOpen bool) error {
		text, extraction = "", textExtraction{}
		var err error
		switch {
		case elementRead:
			text, err = h.extractElementText(tCtx, resolvedTabID, targetFrameID, selectorParam, refParam, modalNodeID)
			h.publishVocabIfReepoched(w, resolvedTabID, vocabBefore)
		case modalOpen:
			err = h.Bridge.CallFunctionOnNode(tCtx, modalNodeID, elementInnerTextJS, nil, &text)
			extraction = textExtraction{Text: text, Mode: extractionRaw, RawLength: utf8.RuneCountInString(text), RawKnown: true}
		default:
			extraction, err = h.extractDocumentText(tCtx, mode, targetFrameID)
		}
		return err
	})
	if errors.Is(scopeErr, errModalUnstable) {
		httpx.Error(w, http.StatusConflict, fmt.Errorf("topmost dialog changed twice during text extraction; retry after the page settles"))
		return
	}
	if scopeErr != nil {
		h.respondSelectorFailure(w, resolvedTabID, scopeErr)
		return
	}
	if workErr != nil {
		if elementRead {
			h.respondSelectorFailure(w, resolvedTabID, fmt.Errorf("element text extract: %w", workErr))
			return
		}
		httpx.Error(w, http.StatusInternalServerError, workErr)
		return
	}
	scopeInfo := h.frameDisclosureFor(tCtx, resolvedTabID, targetFrameID)
	if elementRead {
		h.writeElementTextResponse(w, r, tCtx, text, scopeInfo)
		return
	}
	h.writeTextResponse(w, r, tCtx, extraction, maxChars, format, textRoute, scopeInfo)
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
	"":         "",
	"raw":      "raw",
	"full":     "raw",
	"markdown": "markdown",
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
	extractionMarkdown            = "markdown"
	extractionMarkdownFallback    = "markdown_fallback"
)

// isMarkdownExtraction reports whether an extraction came from the markdown
// mode, so the writer serves text/markdown and truncates on line boundaries.
// The fallback is included: the caller asked for markdown, and raw text is
// still valid Markdown.
func isMarkdownExtraction(mode string) bool {
	return mode == extractionMarkdown || mode == extractionMarkdownFallback
}

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
	Text        string
	Mode        string
	RawLength   int
	RawKnown    bool
	Title       string
	Description string
}

// extractDocumentText reads the document's text (readability unless mode=="raw")
// across all reachable frames, or scoped to targetFrameID when set. Readability
// output that covers too little of the raw document is discarded in favour of
// the raw text, reported as extractionReadabilityFallback.
func (h *Handlers) extractDocumentText(tCtx context.Context, mode, targetFrameID string) (textExtraction, error) {
	if mode == extractionMarkdown {
		return h.extractDocumentMarkdown(tCtx, targetFrameID)
	}
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

// extractDocumentMarkdown converts the frame-scoped rendered document to
// Markdown through the shared seaportal helper, reusing the same HTML read the
// /html inspect path uses so the frame scope is honoured identically. An empty
// conversion falls back to the raw document text, echoed as markdown_fallback.
func (h *Handlers) extractDocumentMarkdown(tCtx context.Context, targetFrameID string) (textExtraction, error) {
	payload, err := h.inspectDocument(tCtx, targetFrameID, inspectKindHTML)
	if err != nil {
		return textExtraction{}, err
	}
	md := scrape.ToMarkdown(payload.HTML, payload.URL)
	if md.Markdown == "" {
		raw, rawErr := h.extractText(tCtx, rawTextScript, targetFrameID)
		if rawErr != nil {
			return textExtraction{}, rawErr
		}
		return textExtraction{
			Text:        raw,
			Mode:        extractionMarkdownFallback,
			RawLength:   utf8.RuneCountInString(raw),
			RawKnown:    true,
			Title:       md.Title,
			Description: md.Description,
		}, nil
	}
	return textExtraction{
		Text:        md.Markdown,
		Mode:        extractionMarkdown,
		Title:       md.Title,
		Description: md.Description,
	}, nil
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

// truncateCharsLine cuts s to at most limit characters, keeping whole lines
// while they fit and then rune-cutting the first line that overruns. A cut never
// splits a Markdown table row or link: a table row (starts with '|') that does
// not fit is dropped whole, and a cut that would land inside a [..](..) link is
// pulled back to before the link. Reports whether it cut. This differs from
// truncateChars, which cuts at the exact rune regardless of line structure.
func truncateCharsLine(s string, limit int) (string, bool) {
	if limit < 0 || utf8.RuneCountInString(s) <= limit {
		return s, false
	}
	var b strings.Builder
	count := 0
	for i, line := range strings.Split(s, "\n") {
		sep := 0
		if i > 0 {
			sep = 1 // the newline joining this line to the previous
		}
		lineLen := utf8.RuneCountInString(line)
		if count+sep+lineLen <= limit {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
			count += sep + lineLen
			continue
		}
		// This line overruns. Keep a rune-cut of it unless its shape forbids
		// splitting, in which case the whole line is dropped.
		budget := limit - count - sep
		if budget > 0 && !strings.HasPrefix(line, "|") {
			if partial, ok := cutLineForMarkdown(line, budget); ok {
				if i > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(partial)
			}
		}
		break
	}
	return b.String(), true
}

// markdownLinkRe matches a whole [text](url) link so a truncation can refuse to
// cut through one.
var markdownLinkRe = regexp.MustCompile(`\[[^\]]*\]\([^)]*\)`)

// cutLineForMarkdown returns the leading runes of line (at most budget) for use
// as the final partial line of a truncation. A cut that would fall inside a
// [..](..) link is pulled back to the link's start so no half-open link escapes,
// then trailing whitespace is trimmed. ok is false only when nothing survives —
// the line began with the link that the budget split.
func cutLineForMarkdown(line string, budget int) (string, bool) {
	cut := len(line)
	count := 0
	for off := range line {
		if count == budget {
			cut = off
			break
		}
		count++
	}
	for _, loc := range markdownLinkRe.FindAllStringIndex(line, -1) {
		if loc[0] < cut && cut < loc[1] {
			cut = loc[0]
			break
		}
	}
	partial := strings.TrimRight(line[:cut], " \t")
	if partial == "" {
		return "", false
	}
	return partial, true
}

// writeTextResponse truncates, IDPI-scans, and writes the document text as
// plain text (format text/plain) or the JSON envelope.
func (h *Handlers) writeTextResponse(w http.ResponseWriter, r *http.Request, tCtx context.Context, extraction textExtraction, maxChars int, format string, route *browserops.RouteMetadata, scope *frameDisclosure) {
	text := extraction.Text
	truncated := false
	if maxChars > -1 {
		if isMarkdownExtraction(extraction.Mode) {
			text, truncated = truncateCharsLine(text, maxChars)
		} else {
			text, truncated = truncateChars(text, maxChars)
		}
	}

	url, _ := h.Bridge.CurrentURL(tCtx)
	title, _ := h.Bridge.CurrentTitle(tCtx)
	if extraction.Title != "" {
		title = extraction.Title
	}
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
		contentType := "text/plain; charset=utf-8"
		if isMarkdownExtraction(extraction.Mode) {
			contentType = "text/markdown; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
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
	if extraction.Description != "" {
		resp["description"] = extraction.Description
	}
	if result.Warning != "" {
		resp["idpiWarning"] = result.Warning
	}
	httpx.JSON(w, 200, resp)
}

// @Endpoint GET /tabs/{id}/text
// @Param mode string query Extraction mode: "" default Readability, "raw"/"full" whole-page innerText, "markdown" seaportal Markdown of the rendered page (falls back to raw text as "markdown_fallback" when the converter yields nothing); any other value is a 400 (optional)
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

const elementInnerTextJS = `function() { return this.innerText || this.textContent || ''; }`

func (h *Handlers) extractElementText(ctx context.Context, tabID, frameID, selectorValue, ref string, scopeBackendNodeID int64) (string, error) {
	raw := selectorValue
	if ref != "" {
		raw = "ref:" + ref
	}
	nodeID, err := h.resolveTextTargetNodeID(ctx, tabID, frameID, raw, scopeBackendNodeID)
	if err != nil {
		return "", err
	}
	if nodeID == 0 {
		return "", fmt.Errorf("no element matches selector %q: %w", raw, bridge.ErrSelectorNoMatch)
	}
	var text string
	if err := h.Bridge.CallFunctionOnNode(ctx, nodeID, elementInnerTextJS, nil, &text); err != nil {
		return "", err
	}
	return text, nil
}

func (h *Handlers) resolveTextTargetNodeID(ctx context.Context, tabID, frameID, raw string, scopeBackendNodeID int64) (int64, error) {
	if scopeBackendNodeID != 0 {
		return bridge.ResolveUnifiedSelectorWithinNode(ctx, selector.Parse(raw), h.Bridge.GetRefCache(tabID), scopeBackendNodeID)
	}
	return h.resolveSelectorNodeIDInFrame(ctx, tabID, raw, frameID)
}

const (
	readyStateWaitTimeout  = 5 * time.Second
	readyStatePollInterval = 100 * time.Millisecond
)

func (h *Handlers) waitForReadyState(ctx context.Context) {
	wctx, cancel := context.WithTimeout(ctx, readyStateWaitTimeout)
	defer cancel()
	// An eval error or a non-loading state both stop the wait, matching the
	// original loop which returned (silently) on either condition.
	_ = pollUntil(wctx, readyStatePollInterval, func() (bool, error) {
		var state string
		if err := h.Bridge.Evaluate(wctx, `document.readyState`, &state, bridge.EvalOpts{}); err != nil {
			return true, nil
		}
		return state != "loading", nil
	})
}
