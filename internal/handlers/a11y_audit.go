package handlers

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/audit"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// @Endpoint GET /a11y/audit
func (h *Handlers) HandleA11yAudit(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")

	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}
	h.recordReadRequest(r, "a11y_audit", tabID)

	resolvedTabID, tCtx, cancel, ok := h.resolveReadContext(w, r, tabID, h.Config.ActionTimeout)
	if !ok {
		return
	}
	defer h.armAutoCloseIfEnabled(resolvedTabID)
	defer cancel()

	h.waitForReadyState(tCtx)

	rawNodes, err := bridge.FetchAXTree(tCtx)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("fetch accessibility tree: %w", err))
		return
	}
	nodes, _ := bridge.BuildSnapshot(rawNodes, "", -1)

	var facts audit.PageFacts
	if err := h.Bridge.Evaluate(tCtx, audit.PageFactsScript, &facts, bridge.EvalOpts{}); err != nil {
		httpx.Error(w, 500, fmt.Errorf("collect page facts: %w", err))
		return
	}

	report := audit.EvaluateA11y(nodes, facts)
	httpx.JSON(w, 200, struct {
		TabID string `json:"tabId"`
		audit.A11yReport
	}{resolvedTabID, report})
}

// @Endpoint GET /tabs/{id}/a11y/audit
func (h *Handlers) HandleTabA11yAudit(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleA11yAudit)
}
