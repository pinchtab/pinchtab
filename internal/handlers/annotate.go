package handlers

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// @Endpoint GET /annotate
//
// HandleAnnotate injects a persistent, clickable annotation overlay onto the
// current page: one pink box per interactive element, each labelled with its
// ref. Unlike `screenshot?annotate=true` (which bakes boxes into an image and
// removes them), this overlay stays on the live page — intended for headed
// browsers where a human clicks a label to copy a reference block for that
// element. `?clear=true` removes the overlay instead.
func (h *Handlers) HandleAnnotate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}

	q := r.URL.Query()
	tabID := q.Get("tabId")
	selector := q.Get("selector")
	clear := q.Get("clear") == "true" || q.Get("clear") == "1"

	resolvedTabID, tCtx, cancel, ok := h.resolveBinaryReadContext(w, r, tabID, h.Config.ActionTimeout)
	if !ok {
		return
	}
	defer cancel()

	if clear {
		if err := cdptk.RemoveInteractiveOverlay(tCtx); err != nil {
			httpx.Error(w, 500, fmt.Errorf("annotate clear: %w", err))
			return
		}
		httpx.JSON(w, 200, map[string]any{"cleared": true})
		return
	}

	// Validate the selector up front so a bad selector surfaces as a 400 (user
	// error), matching the /screenshot and /capture convention, rather than the
	// generic 500 the collector would otherwise produce.
	if selector != "" {
		if _, sErr := h.resolveSelectorNodeID(tCtx, resolvedTabID, selector); sErr != nil {
			httpx.Error(w, 400, frameScopedSelectorError("selector", sErr))
			return
		}
	}

	// Reuse the screenshot annotation collector: it records refs in the tab's
	// ref cache (so a follow-up `click e5` resolves the same node) and returns
	// viewport-relative rects. We keep every candidate — not just the visible
	// viewport — so scrolling the page reveals boxes across the whole document.
	items, _, err := h.collectScreenshotAnnotations(tCtx, resolvedTabID, selector)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("annotate: %w", err))
		return
	}
	if err := cdptk.InjectInteractiveOverlay(tCtx, items); err != nil {
		httpx.Error(w, 500, fmt.Errorf("annotate inject: %w", err))
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"annotated":   len(items),
		"annotations": items,
	})
}

// @Endpoint GET /tabs/{id}/annotate
func (h *Handlers) HandleTabAnnotate(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleAnnotate)
}
