package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (h *Handlers) resolveConsoleTab(w http.ResponseWriter, r *http.Request) (context.Context, string, bool) {
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		ctx, resolvedID, err := h.Bridge.TabContext("")
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err)
			return nil, "", false
		}
		return ctx, resolvedID, true
	}

	ctx, resolvedID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, fmt.Errorf("tab not found"))
		return nil, "", false
	}
	return ctx, resolvedID, true
}

// HandleGetConsoleLogs returns console logs for a tab.
func (h *Handlers) HandleGetConsoleLogs(w http.ResponseWriter, r *http.Request) {
	ctx, tabID, ok := h.resolveConsoleTab(w, r)
	if !ok {
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, tabID); !ok {
		return
	}

	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}

	logs := h.Bridge.GetConsoleLogs(tabID, limit)
	if logs == nil {
		logs = make([]bridge.LogEntry, 0)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"tabId":   tabID,
		"console": logs,
	})
}

// HandleClearConsoleLogs clears console logs for a tab.
func (h *Handlers) HandleClearConsoleLogs(w http.ResponseWriter, r *http.Request) {
	_, tabID, ok := h.resolveConsoleTab(w, r)
	if !ok {
		return
	}

	h.Bridge.ClearConsoleLogs(tabID)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tabId":   tabID,
	})
}

// HandleGetErrorLogs returns error logs for a tab.
func (h *Handlers) HandleGetErrorLogs(w http.ResponseWriter, r *http.Request) {
	ctx, tabID, ok := h.resolveConsoleTab(w, r)
	if !ok {
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, tabID); !ok {
		return
	}

	const maxErrorLogLimit = 1000
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			if v < 0 {
				v = 0
			} else if v > maxErrorLogLimit {
				v = maxErrorLogLimit
			}
			limit = v
		}
	}

	errors := h.Bridge.GetErrorLogs(tabID, limit)
	if errors == nil {
		errors = make([]bridge.ErrorEntry, 0)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"tabId":  tabID,
		"errors": errors,
	})
}

// HandleClearErrorLogs clears error logs for a tab.
func (h *Handlers) HandleClearErrorLogs(w http.ResponseWriter, r *http.Request) {
	_, tabID, ok := h.resolveConsoleTab(w, r)
	if !ok {
		return
	}

	h.Bridge.ClearErrorLogs(tabID)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tabId":   tabID,
	})
}
