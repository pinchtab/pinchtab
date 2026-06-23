package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type offlineRequest struct {
	TabID              string  `json:"tabId"`
	Offline            bool    `json:"offline"`
	Latency            float64 `json:"latency"`
	DownloadThroughput float64 `json:"downloadThroughput"`
	UploadThroughput   float64 `json:"uploadThroughput"`
}

// HandleSetOffline enables or disables network offline emulation via CDP.
// POST /emulation/offline
func (h *Handlers) HandleSetOffline(w http.ResponseWriter, r *http.Request) {
	var req offlineRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	h.setOffline(w, r, req)
}

// HandleTabSetOffline enables or disables network offline emulation for a specific tab.
// POST /tabs/{id}/emulation/offline
func (h *Handlers) HandleTabSetOffline(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("missing tab ID"))
		return
	}

	var req offlineRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.TabID != "" && req.TabID != tabID {
		httpx.Error(w, 400, fmt.Errorf("tabId in body %q does not match URL path %q", req.TabID, tabID))
		return
	}
	req.TabID = tabID

	h.setOffline(w, r, req)
}

func (h *Handlers) setOffline(w http.ResponseWriter, r *http.Request, req offlineRequest) {
	// Apply defaults for throughput: -1 means no throttling.
	if req.DownloadThroughput == 0 {
		req.DownloadThroughput = -1
	}
	if req.UploadThroughput == 0 {
		req.UploadThroughput = -1
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

	if err := h.Bridge.SetNetworkConditions(tCtx, bridge.NetworkConditions{
		Offline:            req.Offline,
		Latency:            req.Latency,
		DownloadThroughput: req.DownloadThroughput,
		UploadThroughput:   req.UploadThroughput,
	}); err != nil {
		httpx.Error(w, 500, fmt.Errorf("CDP network offline emulation: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "emulation.offline", TabID: resolvedTabID})

	status := "online"
	if req.Offline {
		status = "offline"
	}

	httpx.JSON(w, 200, map[string]any{
		"offline": req.Offline,
		"status":  status,
	})
}
