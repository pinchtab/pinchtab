package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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

	ctx, resolvedID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	accept := req.Action == "accept"
	h.handleDialogAction(w, r, ctx, resolvedID, accept, req.Text)
}

// HandleTabDialog handles the current JavaScript dialog for a specific tab.
//
// @Endpoint POST /tabs/{id}/dialog
func (h *Handlers) HandleTabDialog(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			httpx.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	body["tabId"] = tabID

	payload, err := json.Marshal(body)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.HandleDialog(w, req)
}

func (h *Handlers) handleDialogAction(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID string, accept bool, promptText string) {
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
