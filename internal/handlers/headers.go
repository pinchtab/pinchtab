package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type headersRequest struct {
	TabID   string            `json:"tabId"`
	Headers map[string]string `json:"headers"`
}

func (h *Handlers) HandleSetHeaders(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSONBody[headersRequest](w, r)
	if !ok {
		return
	}

	h.setHeaders(w, r, req)
}

func (h *Handlers) HandleTabSetHeaders(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSONBody[headersRequest](w, r)
	if !ok {
		return
	}
	tabID, ok := h.requirePathTabIDMatch(w, r, req.TabID)
	if !ok {
		return
	}
	req.TabID = tabID

	h.setHeaders(w, r, req)
}

func (h *Handlers) setHeaders(w http.ResponseWriter, r *http.Request, req headersRequest) {
	if req.Headers == nil {
		httpx.Error(w, 400, fmt.Errorf("missing required field: headers"))
		return
	}

	ctx, resolvedTabID, ok := h.guardedTabContext(w, r, req.TabID, guardDomainPolicy|guardHandoffPause)
	if !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tCancel()

	if err := h.Bridge.SetExtraHTTPHeaders(tCtx, req.Headers); err != nil {
		httpx.Error(w, 500, fmt.Errorf("CDP set extra HTTP headers: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "emulation.headers", TabID: resolvedTabID})

	httpx.JSON(w, 200, map[string]any{
		"headers": req.Headers,
		"status":  "applied",
	})
}
