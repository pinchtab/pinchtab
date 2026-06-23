package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type tabLoadState struct {
	ReadyState           string `json:"readyState,omitempty"`
	NavigationInProgress bool   `json:"navigationInProgress"`
	NetworkIdle          *bool  `json:"networkIdle,omitempty"`
	State                string `json:"state"`
}

type tabStateResponse struct {
	TabID         string       `json:"tabId"`
	URL           string       `json:"url,omitempty"`
	Title         string       `json:"title,omitempty"`
	DialogPresent bool         `json:"dialogPresent"`
	Dialog        interface{}  `json:"dialog,omitempty"`
	Load          tabLoadState `json:"load"`
	Actionability string       `json:"actionability"`
}

// HandleTabState returns lightweight tab/page state signals for agent workflows.
func (h *Handlers) HandleTabState(w http.ResponseWriter, r *http.Request) {
	if h.Bridge == nil {
		httpx.ErrorCode(w, http.StatusServiceUnavailable, "bridge_unavailable", "browser bridge unavailable", false, nil)
		return
	}

	tabID := r.PathValue("id")
	if tabID == "" {
		tabID = r.PathValue("tabId")
	}
	if tabID == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_tab_id", "missing tab id", false, nil)
		return
	}

	_, resolvedTabID, err := h.Bridge.TabContext(tabID)
	if err != nil {
		WriteTabContextError(w, err, http.StatusNotFound)
		return
	}

	resp := tabStateResponse{
		TabID:         resolvedTabID,
		DialogPresent: false,
		Load:          tabLoadState{State: "unknown"},
		Actionability: "ready",
	}

	if targets, err := h.Bridge.ListTargets(); err == nil {
		for _, t := range targets {
			if t.TargetID == resolvedTabID {
				resp.URL = t.URL
				resp.Title = t.Title
				break
			}
		}
	}

	if dm := h.Bridge.GetDialogManager(); dm != nil {
		if dialog := dm.GetPending(resolvedTabID); dialog != nil {
			resp.Dialog = dialog
			resp.DialogPresent = true
			resp.Actionability = "blocked"
		}
	}

	if bridgeWithState, ok := h.Bridge.(interface {
		GetDocumentReadyState(string) (string, error)
		IsNetworkIdle(string) (bool, bool)
	}); ok {
		if readyState, err := bridgeWithState.GetDocumentReadyState(resolvedTabID); err == nil {
			resp.Load.ReadyState = readyState
			switch readyState {
			case "loading":
				resp.Load.State = "loading"
				resp.Load.NavigationInProgress = true
				if resp.Actionability == "ready" {
					resp.Actionability = "caution"
				}
			case "interactive":
				resp.Load.State = "interactive"
				if resp.Actionability == "ready" {
					resp.Actionability = "caution"
				}
			case "complete":
				resp.Load.State = "complete"
			}
		}
		if idle, ok := bridgeWithState.IsNetworkIdle(resolvedTabID); ok {
			resp.Load.NetworkIdle = &idle
			if !idle && resp.Load.State == "complete" {
				resp.Load.State = "busy"
				if resp.Actionability == "ready" {
					resp.Actionability = "caution"
				}
			}
		}
	}

	httpx.JSON(w, http.StatusOK, resp)
}
