package handlers

import (
	"context"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

func (h *Handlers) tabContext(r *http.Request, tabID string) (context.Context, string, error) {
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err == nil {
		h.recordActivity(r, activity.Update{TabID: resolvedID})
	}
	return ctx, resolvedID, err
}

// tabContextWithHeader resolves the tab and sets the resolved tab ID as a
// response header so the orchestrator proxy can enrich activity events
// even for large responses (e.g. snapshots) that exceed the body-inspection threshold.
func (h *Handlers) tabContextWithHeader(w http.ResponseWriter, r *http.Request, tabID string) (context.Context, string, error) {
	ctx, resolvedID, err := h.tabContext(r, tabID)
	if err == nil {
		w.Header().Set(activity.HeaderPTTabID, resolvedID)
	}
	return ctx, resolvedID, err
}

func (h *Handlers) recordActivity(r *http.Request, update activity.Update) {
	activity.EnrichRequest(r, update)
}

func (h *Handlers) recordNavigateRequest(r *http.Request, tabID, url string) {
	h.recordActivity(r, activity.Update{
		Action: "navigate",
		TabID:  tabID,
		URL:    url,
	})
}

func (h *Handlers) recordActionRequest(r *http.Request, req bridge.ActionRequest) {
	h.recordActivity(r, activity.Update{
		Action: req.Kind,
		TabID:  req.TabID,
		Ref:    req.Ref,
	})
}

func (h *Handlers) recordReadRequest(r *http.Request, action, tabID string) {
	h.recordActivity(r, activity.Update{
		Action: action,
		TabID:  tabID,
	})
}

func (h *Handlers) recordResolvedURL(r *http.Request, url string) {
	h.recordActivity(r, activity.Update{URL: url})
}

func (h *Handlers) recordEngine(r *http.Request, engine string) {
	h.recordActivity(r, activity.Update{Engine: engine})
}

func (h *Handlers) recordResolvedTab(r *http.Request, tabID string) {
	h.recordActivity(r, activity.Update{TabID: tabID})
}
