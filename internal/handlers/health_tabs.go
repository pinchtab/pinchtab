package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	targets, err := h.Bridge.ListTargets()
	if err != nil {
		web.JSON(w, 200, map[string]any{"status": "disconnected", "error": err.Error(), "cdp": h.Config.CdpURL})
		return
	}
	web.JSON(w, 200, map[string]any{"status": "ok", "tabs": len(targets), "cdp": h.Config.CdpURL})
}

func (h *Handlers) HandleEnsureChrome(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized for this instance
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization failed: %w", err))
		return
	}
	web.JSON(w, 200, map[string]string{"status": "chrome_ready"})
}

func (h *Handlers) HandleTabs(w http.ResponseWriter, r *http.Request) {
	targets, err := h.Bridge.ListTargets()
	if err != nil {
		web.Error(w, 500, err)
		return
	}

	tabs := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		entry := map[string]any{
			"id":    string(t.TargetID),
			"url":   t.URL,
			"title": t.Title,
			"type":  t.Type,
		}
		if lock := h.Bridge.TabLockInfo(string(t.TargetID)); lock != nil {
			entry["owner"] = lock.Owner
			entry["lockedUntil"] = lock.ExpiresAt.Format(time.RFC3339)
		}
		tabs = append(tabs, entry)
	}
	web.JSON(w, 200, map[string]any{"tabs": tabs})
}
