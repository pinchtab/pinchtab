package handlers

import (
	"encoding/json"
	"fmt"
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
