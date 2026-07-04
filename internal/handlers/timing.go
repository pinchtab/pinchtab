package handlers

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// @Endpoint GET /timing
func (h *Handlers) HandleTiming(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")

	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}
	h.recordReadRequest(r, "timing", tabID)

	resolvedTabID, tCtx, cancel, ok := h.resolveReadContext(w, r, tabID, h.Config.ActionTimeout)
	if !ok {
		return
	}
	defer h.armAutoCloseIfEnabled(resolvedTabID)
	defer cancel()

	// Paint metrics (FCP/LCP) only fire on visible pages; background tabs
	// stay hidden in headless Chrome, so surface the tab before measuring.
	_ = h.Bridge.FocusTab(resolvedTabID)

	metrics, err := observe.CollectTiming(func(expression string, result any) error {
		return h.Bridge.Evaluate(tCtx, expression, result, bridge.EvalOpts{AwaitPromise: true})
	})
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("collect timing: %w", err))
		return
	}

	httpx.JSON(w, 200, struct {
		TabID string `json:"tabId"`
		*observe.TimingMetrics
	}{resolvedTabID, metrics})
}

// @Endpoint GET /tabs/{id}/timing
func (h *Handlers) HandleTabTiming(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleTiming)
}
