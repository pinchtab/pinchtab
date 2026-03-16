package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
)

func (h *Handlers) HandleGetConsoleLogs(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		_, resolvedID, err := h.Bridge.TabContext("")
		if err != nil {
			web.Error(w, http.StatusBadRequest, err)
			return
		}
		tabID = resolvedID
	} else {
		_, resolvedID, err := h.Bridge.TabContext(tabID)
		if err != nil {
			web.Error(w, http.StatusNotFound, fmt.Errorf("Tab not found"))
			return
		}
		tabID = resolvedID
	}

	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}

	logs := h.Bridge.GetConsoleLogs(tabID, limit)
	if logs == nil {
		logs = make([]bridge.LogEntry, 0) // return empty list instead of null if none
	}
	web.JSON(w, http.StatusOK, map[string]any{
		"tabId":   tabID,
		"console": logs,
	})
}

func (h *Handlers) HandleClearConsoleLogs(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		_, resolvedID, err := h.Bridge.TabContext("")
		if err != nil {
			web.Error(w, http.StatusBadRequest, err)
			return
		}
		tabID = resolvedID
	} else {
		_, resolvedID, err := h.Bridge.TabContext(tabID)
		if err != nil {
			web.Error(w, http.StatusNotFound, fmt.Errorf("Tab not found"))
			return
		}
		tabID = resolvedID
	}

	h.Bridge.ClearConsoleLogs(tabID)
	web.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tabId":   tabID,
	})
}

func (h *Handlers) HandleGetErrorLogs(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		_, resolvedID, err := h.Bridge.TabContext("")
		if err != nil {
			web.Error(w, http.StatusBadRequest, err)
			return
		}
		tabID = resolvedID
	} else {
		_, resolvedID, err := h.Bridge.TabContext(tabID)
		if err != nil {
			web.Error(w, http.StatusNotFound, fmt.Errorf("Tab not found"))
			return
		}
		tabID = resolvedID
	}

	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}

	errors := h.Bridge.GetErrorLogs(tabID, limit)
	if errors == nil {
		errors = make([]bridge.ErrorEntry, 0)
	}
	web.JSON(w, http.StatusOK, map[string]any{
		"tabId":  tabID,
		"errors": errors,
	})
}

func (h *Handlers) HandleClearErrorLogs(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		_, resolvedID, err := h.Bridge.TabContext("")
		if err != nil {
			web.Error(w, http.StatusBadRequest, err)
			return
		}
		tabID = resolvedID
	} else {
		_, resolvedID, err := h.Bridge.TabContext(tabID)
		if err != nil {
			web.Error(w, http.StatusNotFound, fmt.Errorf("Tab not found"))
			return
		}
		tabID = resolvedID
	}

	h.Bridge.ClearErrorLogs(tabID)
	web.JSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tabId":   tabID,
	})
}
