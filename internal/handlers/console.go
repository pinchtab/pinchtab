package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const maxLogLimit = 1000

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

func parseLogLimit(raw string) int {
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > maxLogLimit {
		return maxLogLimit
	}
	return v
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

	limit := parseLogLimit(r.URL.Query().Get("limit"))

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

	limit := parseLogLimit(r.URL.Query().Get("limit"))

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
