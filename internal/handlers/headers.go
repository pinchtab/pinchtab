package handlers

import (
	"context"
	"encoding/json"
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

// HandleSetHeaders sets extra HTTP headers via CDP.
// POST /emulation/headers
func (h *Handlers) HandleSetHeaders(w http.ResponseWriter, r *http.Request) {
	var req headersRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	h.setHeaders(w, r, req)
}

// HandleTabSetHeaders sets extra HTTP headers for a specific tab.
// POST /tabs/{id}/emulation/headers
func (h *Handlers) HandleTabSetHeaders(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("missing tab ID"))
		return
	}

	var req headersRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.TabID != "" && req.TabID != tabID {
		httpx.Error(w, 400, fmt.Errorf("tabId in body %q does not match URL path %q", req.TabID, tabID))
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

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
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
