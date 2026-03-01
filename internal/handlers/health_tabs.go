package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	// Guard against nil Bridge
	if h.Bridge == nil {
		web.JSON(w, 503, map[string]any{"status": "error", "reason": "bridge not initialized"})
		return
	}

	// Ensure Chrome is initialized before checking health
	if err := h.ensureChrome(); err != nil {
		web.JSON(w, 503, map[string]any{"status": "error", "reason": fmt.Sprintf("chrome initialization failed: %v", err)})
		return
	}

	targets, err := h.Bridge.ListTargets()
	if err != nil {
		web.JSON(w, 503, map[string]any{"status": "error", "reason": err.Error()})
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

func (h *Handlers) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, map[string]any{"metrics": snapshotMetrics()})
}

func (h *Handlers) HandleTabs(w http.ResponseWriter, r *http.Request) {
	// Guard against nil Bridge
	if h.Bridge == nil {
		web.Error(w, 503, fmt.Errorf("bridge not initialized"))
		return
	}

	targets, err := h.Bridge.ListTargets()
	if err != nil {
		web.Error(w, 503, err)
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
