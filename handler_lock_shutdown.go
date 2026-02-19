package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (b *Bridge) handleTabLock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID      string `json:"tabId"`
		Owner      string `json:"owner"`
		TimeoutSec int    `json:"timeoutSec"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID == "" || req.Owner == "" {
		jsonErr(w, 400, fmt.Errorf("tabId and owner required"))
		return
	}

	timeout := defaultLockTimeout
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	if err := b.locks.Lock(req.TabID, req.Owner, timeout); err != nil {
		jsonErr(w, 409, err)
		return
	}

	lock := b.locks.Get(req.TabID)
	jsonResp(w, 200, map[string]any{
		"locked":    true,
		"owner":     lock.Owner,
		"expiresAt": lock.ExpiresAt.Format(time.RFC3339),
	})
}

func (b *Bridge) handleTabUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID string `json:"tabId"`
		Owner string `json:"owner"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if req.TabID == "" || req.Owner == "" {
		jsonErr(w, 400, fmt.Errorf("tabId and owner required"))
		return
	}

	if err := b.locks.Unlock(req.TabID, req.Owner); err != nil {
		jsonErr(w, 409, err)
		return
	}

	jsonResp(w, 200, map[string]any{"unlocked": true})
}

func (b *Bridge) handleShutdown(shutdownFn func()) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("shutdown requested via API")
		jsonResp(w, 200, map[string]any{"status": "shutting down"})

		go func() {
			time.Sleep(100 * time.Millisecond)
			shutdownFn()
		}()
	}
}
