package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
)

func (h *Handlers) HandleTabLock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID      string `json:"tabId"`
		Owner      string `json:"owner"`
		TimeoutSec int    `json:"timeoutSec"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID == "" || req.Owner == "" {
		web.Error(w, 400, fmt.Errorf("tabId and owner required"))
		return
	}

	timeout := bridge.DefaultLockTimeout
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	if err := h.Bridge.Lock(req.TabID, req.Owner, timeout); err != nil {
		web.Error(w, 409, err)
		return
	}

	lock := h.Bridge.TabLockInfo(req.TabID)
	web.JSON(w, 200, map[string]any{
		"locked":    true,
		"owner":     lock.Owner,
		"expiresAt": lock.ExpiresAt.Format(time.RFC3339),
	})
}

func (h *Handlers) HandleTabUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID string `json:"tabId"`
		Owner string `json:"owner"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID == "" || req.Owner == "" {
		web.Error(w, 400, fmt.Errorf("tabId and owner required"))
		return
	}

	if err := h.Bridge.Unlock(req.TabID, req.Owner); err != nil {
		web.Error(w, 409, err)
		return
	}

	web.JSON(w, 200, map[string]any{"unlocked": true})
}

// HandleTabLockByID locks a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/lock
func (h *Handlers) HandleTabLockByID(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			web.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			web.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	body["tabId"] = tabID

	payload, err := json.Marshal(body)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.HandleTabLock(w, req)
}

// HandleTabUnlockByID unlocks a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/unlock
func (h *Handlers) HandleTabUnlockByID(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			web.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			web.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	body["tabId"] = tabID

	payload, err := json.Marshal(body)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.HandleTabUnlock(w, req)
}

func (h *Handlers) HandleShutdown(shutdownFn func()) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("shutdown requested via API")
		web.JSON(w, 200, map[string]any{"status": "shutting down"})

		go func() {
			time.Sleep(100 * time.Millisecond)
			shutdownFn()
		}()
	}
}
