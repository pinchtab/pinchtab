package handlers

import (
	"context"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

// HandleTabBack navigates a specific tab back in history.
func (h *Handlers) HandleTabBack(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleBack)
}

// HandleTabForward navigates a specific tab forward in history.
func (h *Handlers) HandleTabForward(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleForward)
}

// HandleTabReload reloads a specific tab.
func (h *Handlers) HandleTabReload(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.HandleReload)
}

// handleHistoryNav runs the shared back/forward/reload flow: browser + tab
// resolution, then navigate (which performs the per-op settle), then the
// current-URL JSON response. navigate receives the resolved tab ctx/id and the
// dismissBanners flag and returns an error mapped to 500.
func (h *Handlers) handleHistoryNav(w http.ResponseWriter, r *http.Request,
	navigate func(ctx context.Context, resolvedID string, dismissBanners bool) error) {
	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	dismissBanners := historyDismissBannersFlag(r)
	ctx, resolvedID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if err := navigate(ctx, resolvedID, dismissBanners); err != nil {
		httpx.Error(w, 500, err)
		return
	}
	curURL, _ := h.Bridge.CurrentURL(ctx)
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}

// historyMove is the back/forward settle: move, then on a real navigation wait
// for the document readyState to leave "loading" (bfcache restores return
// immediately) before running banner dismissal (gated internally by the flag).
func (h *Handlers) historyMove(ctx context.Context, resolvedID string, dismissBanners bool,
	move func(context.Context) (bool, error)) error {
	didNavigate, err := move(ctx)
	if err != nil {
		return err
	}
	if didNavigate {
		h.waitForReadyState(ctx)
		h.dismissBanners(ctx, resolvedID, dismissBanners)
	}
	return nil
}

// HandleBack navigates the current (or specified) tab back in history.
func (h *Handlers) HandleBack(w http.ResponseWriter, r *http.Request) {
	h.handleHistoryNav(w, r, func(ctx context.Context, resolvedID string, dismissBanners bool) error {
		return h.historyMove(ctx, resolvedID, dismissBanners, h.Bridge.GoBack)
	})
}

// HandleForward navigates the current (or specified) tab forward in history.
func (h *Handlers) HandleForward(w http.ResponseWriter, r *http.Request) {
	h.handleHistoryNav(w, r, func(ctx context.Context, resolvedID string, dismissBanners bool) error {
		return h.historyMove(ctx, resolvedID, dismissBanners, h.Bridge.GoForward)
	})
}

// HandleReload reloads the current (or specified) tab.
func (h *Handlers) HandleReload(w http.ResponseWriter, r *http.Request) {
	h.handleHistoryNav(w, r, func(ctx context.Context, resolvedID string, dismissBanners bool) error {
		if err := h.Bridge.Reload(ctx); err != nil {
			return err
		}
		// page.Reload() returns when the nav kicks off, not when the document
		// commits — wait for the new DOM's readyState to leave "loading" before
		// the dismissal helper looks for cookie/consent buttons. Mirrors the
		// readyState wait on back/forward. Skipped when the flag is off so vanilla
		// reloads stay fast; unlike back/forward, reload gates the settle on the
		// flag, not a didNavigate signal.
		if dismissBanners {
			h.waitForReadyState(ctx)
			h.dismissBanners(ctx, resolvedID, true)
		}
		return nil
	})
}

// historyDismissBannersFlag reads the dismissBanners flag from a back/forward/
// reload request. These endpoints don't carry a JSON body, so we accept it as
// a query string parameter only.
func historyDismissBannersFlag(r *http.Request) bool {
	return queryTruthy(r.URL.Query().Get("dismissBanners"))
}
