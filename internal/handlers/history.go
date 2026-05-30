package handlers

import (
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

// HandleTabBack navigates a specific tab back in history.
func (h *Handlers) HandleTabBack(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("tabId", r.PathValue("id"))
	r.URL.RawQuery = q.Encode()
	h.HandleBack(w, r)
}

// HandleTabForward navigates a specific tab forward in history.
func (h *Handlers) HandleTabForward(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("tabId", r.PathValue("id"))
	r.URL.RawQuery = q.Encode()
	h.HandleForward(w, r)
}

// HandleTabReload reloads a specific tab.
func (h *Handlers) HandleTabReload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("tabId", r.PathValue("id"))
	r.URL.RawQuery = q.Encode()
	h.HandleReload(w, r)
}

// HandleBack navigates the current (or specified) tab back in history.
func (h *Handlers) HandleBack(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	dismissBanners := historyDismissBannersFlag(r)
	ctx, resolvedID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}

	didNavigate, err := h.Bridge.GoBack(ctx)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	if didNavigate {
		time.Sleep(200 * time.Millisecond)
		h.dismissBanners(ctx, resolvedID, dismissBanners)
	}

	curURL, _ := h.Bridge.CurrentURL(ctx)
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}

// HandleForward navigates the current (or specified) tab forward in history.
func (h *Handlers) HandleForward(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	dismissBanners := historyDismissBannersFlag(r)
	ctx, resolvedID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}

	didNavigate, err := h.Bridge.GoForward(ctx)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	if didNavigate {
		time.Sleep(200 * time.Millisecond)
		h.dismissBanners(ctx, resolvedID, dismissBanners)
	}

	curURL, _ := h.Bridge.CurrentURL(ctx)
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}

// HandleReload reloads the current (or specified) tab.
func (h *Handlers) HandleReload(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	dismissBanners := historyDismissBannersFlag(r)
	ctx, resolvedID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}

	if err := h.Bridge.Reload(ctx); err != nil {
		httpx.Error(w, 500, err)
		return
	}
	// page.Reload() returns when the nav kicks off, not when the document
	// commits — give the new DOM time to render before the dismissal helper
	// looks for cookie/consent buttons. Mirrors the 200 ms sleep on back/
	// forward. Skipped when the flag is off so vanilla reloads stay fast.
	if dismissBanners {
		time.Sleep(200 * time.Millisecond)
		h.dismissBanners(ctx, resolvedID, true)
	}

	curURL, _ := h.Bridge.CurrentURL(ctx)
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}

// historyDismissBannersFlag reads the dismissBanners flag from a back/forward/
// reload request. These endpoints don't carry a JSON body, so we accept it as
// a query string parameter only.
func historyDismissBannersFlag(r *http.Request) bool {
	v := r.URL.Query().Get("dismissBanners")
	return v == "1" || v == "true" || v == "TRUE" || v == "True"
}
