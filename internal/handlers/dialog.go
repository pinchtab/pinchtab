package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// HandleDialog handles the current JavaScript dialog (accept/dismiss).
//
// @Endpoint POST /dialog
func (h *Handlers) HandleDialog(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID  string `json:"tabId"`
		Action string `json:"action"` // "accept" or "dismiss"
		Text   string `json:"text"`   // optional prompt text
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Action == "" {
		httpx.Error(w, 400, fmt.Errorf("action required (accept or dismiss)"))
		return
	}
	if req.Action != "accept" && req.Action != "dismiss" {
		httpx.Error(w, 400, fmt.Errorf("action must be 'accept' or 'dismiss'"))
		return
	}

	ctx, resolvedID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}

	accept := req.Action == "accept"
	h.handleDialogAction(w, r, ctx, resolvedID, accept, req.Text)
}

// HandleTabDialog handles the current JavaScript dialog for a specific tab.
//
// @Endpoint POST /tabs/{id}/dialog
func (h *Handlers) HandleTabDialog(w http.ResponseWriter, r *http.Request) {
	h.withPathTabIDBody(w, r, h.HandleDialog)
}

func (h *Handlers) handleDialogAction(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID string, accept bool, promptText string) {
	action := "dialog.dismiss"
	if accept {
		action = "dialog.accept"
	}
	h.recordActivity(r, activity.Update{Action: action, TabID: tabID})

	dm := h.Bridge.GetDialogManager()
	if dm == nil {
		httpx.Error(w, 500, fmt.Errorf("dialog manager not available"))
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	result, err := bridge.HandlePendingDialog(tCtx, tabID, dm, accept, promptText)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}

	httpx.JSON(w, 200, result)
}
