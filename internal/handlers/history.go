package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
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
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	// Use CDP directly instead of chromedp.NavigateBack() which wraps in
	// responseAction() and waits for Page.loadEventFired — hangs indefinitely.
	var noHistory bool
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cur, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}
		if cur <= 0 || cur > int64(len(entries)-1) {
			noHistory = true
			return nil
		}
		return page.NavigateToHistoryEntry(entries[cur-1].ID).Do(ctx)
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("back: %w", err))
		return
	}
	if !noHistory {
		time.Sleep(200 * time.Millisecond)
	}

	var curURL string
	_ = chromedp.Run(ctx, chromedp.Location(&curURL))
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}

// HandleForward navigates the current (or specified) tab forward in history.
func (h *Handlers) HandleForward(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	var noHistory bool
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cur, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return fmt.Errorf("get history: %w", err)
		}
		if cur < 0 || cur >= int64(len(entries)-1) {
			noHistory = true
			return nil
		}
		return page.NavigateToHistoryEntry(entries[cur+1].ID).Do(ctx)
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("forward: %w", err))
		return
	}
	if !noHistory {
		time.Sleep(200 * time.Millisecond)
	}

	var curURL string
	_ = chromedp.Run(ctx, chromedp.Location(&curURL))
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}

// HandleReload reloads the current (or specified) tab.
func (h *Handlers) HandleReload(w http.ResponseWriter, r *http.Request) {
	if !h.ensureChromeOrRespond(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.Reload().Do(ctx)
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("reload: %w", err))
		return
	}

	var curURL string
	_ = chromedp.Run(ctx, chromedp.Location(&curURL))
	httpx.JSON(w, 200, map[string]any{"tabId": resolvedID, "url": curURL})
}
