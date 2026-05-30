package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

func (h *Handlers) tabContext(r *http.Request, tabID string) (context.Context, string, error) {
	tabID = strings.TrimSpace(tabID)
	scope := currentTabScopeFromRequest(r)
	explicitTab := tabID != ""

	if !explicitTab && !scope.IsGlobal() {
		storedTabID, ok := h.CurrentTabs.Get(scope)
		if !ok {
			return nil, "", noCurrentTabError(scope.Description())
		}
		tabID = storedTabID
	}

	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil && !explicitTab && !scope.IsGlobal() {
		h.CurrentTabs.Clear(scope)
		return nil, "", noCurrentTabError(scope.Description())
	}
	if err == nil {
		h.setCurrentTabForRequest(r, resolvedID)
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

func (h *Handlers) recordResolvedTab(r *http.Request, tabID string) {
	h.recordActivity(r, activity.Update{TabID: tabID})
}

func (h *Handlers) setCurrentTabForRequest(r *http.Request, tabID string) {
	if h == nil || h.CurrentTabs == nil {
		return
	}
	h.CurrentTabs.Set(currentTabScopeFromRequest(r), tabID)
}

func (h *Handlers) clearCurrentTabReferences(tabID string) {
	if h == nil || h.CurrentTabs == nil {
		return
	}
	h.CurrentTabs.ClearTab(tabID)
}

func (h *Handlers) scopedCurrentTabForRequest(r *http.Request) (string, bool) {
	if h == nil || h.CurrentTabs == nil {
		return "", false
	}
	return h.CurrentTabs.Get(currentTabScopeFromRequest(r))
}
