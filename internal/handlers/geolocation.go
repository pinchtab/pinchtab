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

type geolocationRequest struct {
	TabID     string  `json:"tabId"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy"`
}

// HandleSetGeolocation sets the browser geolocation via CDP emulation.
// POST /emulation/geolocation
func (h *Handlers) HandleSetGeolocation(w http.ResponseWriter, r *http.Request) {
	var req geolocationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	h.setGeolocation(w, r, req)
}

// HandleTabSetGeolocation sets the browser geolocation for a specific tab.
// POST /tabs/{id}/emulation/geolocation
func (h *Handlers) HandleTabSetGeolocation(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("missing tab ID"))
		return
	}

	var req geolocationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.TabID != "" && req.TabID != tabID {
		httpx.Error(w, 400, fmt.Errorf("tabId in body %q does not match URL path %q", req.TabID, tabID))
		return
	}
	req.TabID = tabID

	h.setGeolocation(w, r, req)
}

func (h *Handlers) setGeolocation(w http.ResponseWriter, r *http.Request, req geolocationRequest) {
	if req.Latitude < -90 || req.Latitude > 90 {
		httpx.Error(w, 400, fmt.Errorf("latitude must be between -90 and 90"))
		return
	}

	if req.Longitude < -180 || req.Longitude > 180 {
		httpx.Error(w, 400, fmt.Errorf("longitude must be between -180 and 180"))
		return
	}

	if req.Accuracy < 0 {
		httpx.Error(w, 400, fmt.Errorf("accuracy must be >= 0"))
		return
	}

	if req.Accuracy == 0 {
		req.Accuracy = 1.0
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

	if err := h.Bridge.SetGeolocation(tCtx, req.Latitude, req.Longitude, req.Accuracy); err != nil {
		httpx.Error(w, 500, fmt.Errorf("CDP geolocation override: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "emulation.geolocation", TabID: resolvedTabID})

	httpx.JSON(w, 200, map[string]any{
		"latitude":  req.Latitude,
		"longitude": req.Longitude,
		"accuracy":  req.Accuracy,
		"status":    "applied",
	})
}
