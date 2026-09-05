package handlers

import (
	"context"
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

func (h *Handlers) HandleSetOffline(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSONBody[offlineRequest](w, r)
	if !ok {
		return
	}

	h.setOffline(w, r, req)
}

func (h *Handlers) HandleTabSetOffline(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSONBody[offlineRequest](w, r)
	if !ok {
		return
	}
	tabID, ok := h.requirePathTabIDMatch(w, r, req.TabID)
	if !ok {
		return
	}
	req.TabID = tabID

	h.setOffline(w, r, req)
}

func (h *Handlers) setOffline(w http.ResponseWriter, r *http.Request, req offlineRequest) {
	if req.DownloadThroughput == 0 {
		req.DownloadThroughput = -1
	}
	if req.UploadThroughput == 0 {
		req.UploadThroughput = -1
	}

	ctx, resolvedTabID, ok := h.guardedTabContext(w, r, req.TabID, guardDomainPolicy|guardHandoffPause)
	if !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tCancel()

	if err := h.Bridge.SetNetworkConditions(tCtx, resolvedTabID, bridge.NetworkConditions{
		Offline:            req.Offline,
		Latency:            req.Latency,
		DownloadThroughput: req.DownloadThroughput,
		UploadThroughput:   req.UploadThroughput,
	}); err != nil {
		httpx.Error(w, 500, fmt.Errorf("network offline emulation: %w", err))
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
