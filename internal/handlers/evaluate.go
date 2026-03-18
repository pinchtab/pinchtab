package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (h *Handlers) evaluateEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowEvaluate
}

// HandleEvaluate runs JavaScript in the current tab.
//
// @Endpoint POST /evaluate
func (h *Handlers) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	if !h.evaluateEnabled() {
		httpx.ErrorCode(w, 403, "evaluate_disabled", httpx.DisabledEndpointMessage("evaluate", "security.allowEvaluate"), false, map[string]any{
			"setting": "security.allowEvaluate",
		})
		return
	}

	var req struct {
		TabID      string `json:"tabId"`
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.Expression == "" {
		httpx.Error(w, 400, fmt.Errorf("expression required"))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	slog.Warn("evaluate",
		"tabId", req.TabID,
		"expressionLength", len(req.Expression),
		"remoteAddr", r.RemoteAddr,
	)

	var result any
	if err := chromedp.Run(tCtx, chromedp.Evaluate(req.Expression, &result)); err != nil {
		httpx.Error(w, 500, fmt.Errorf("evaluate: %w", err))
		return
	}

	httpx.JSON(w, 200, map[string]any{"result": result})
}

// HandleTabEvaluate runs JavaScript in a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/evaluate
func (h *Handlers) HandleTabEvaluate(w http.ResponseWriter, r *http.Request) {
	if !h.evaluateEnabled() {
		httpx.ErrorCode(w, 403, "evaluate_disabled", httpx.DisabledEndpointMessage("evaluate", "security.allowEvaluate"), false, map[string]any{
			"setting": "security.allowEvaluate",
		})
		return
	}

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
	h.HandleEvaluate(w, req)
}
