package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
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

	resp := map[string]any{"status": "ok", "tabs": len(targets)}

	// Include crash logs if any
	if crashLogs := h.Bridge.GetCrashLogs(); len(crashLogs) > 0 {
		resp["crashLogs"] = crashLogs
	}
	if hasFailureDiagnostics() {
		resp["failures"] = FailureSnapshot()
	}
	if bridge.HasCrashDiagnostics() {
		resp["crashes"] = bridge.CrashSnapshot()
	}

	web.JSON(w, 200, resp)
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
	result := map[string]any{"metrics": SnapshotMetrics()}
	if hasFailureDiagnostics() {
		result["failures"] = FailureSnapshot()
	}
	if bridge.HasCrashDiagnostics() {
		result["crashes"] = bridge.CrashSnapshot()
	}

	// Aggregate memory metrics across all tabs
	if h.Bridge != nil {
		if mem, err := h.Bridge.GetAggregatedMemoryMetrics(); err == nil && mem != nil {
			result["memory"] = mem
		}
	}

	web.JSON(w, 200, result)
}

func (h *Handlers) HandleTabMetrics(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		web.Error(w, 400, fmt.Errorf("missing tab id"))
		return
	}

	if h.Bridge == nil {
		web.Error(w, 503, fmt.Errorf("bridge not initialized"))
		return
	}

	mem, err := h.Bridge.GetMemoryMetrics(tabID)
	if err != nil {
		web.Error(w, 500, fmt.Errorf("failed to get metrics: %w", err))
		return
	}

	web.JSON(w, 200, mem)
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
		tabID := string(t.TargetID)
		entry := map[string]any{
			"id":    tabID,
			"url":   t.URL,
			"title": t.Title,
			"type":  t.Type,
		}
		if lock := h.Bridge.TabLockInfo(tabID); lock != nil {
			entry["owner"] = lock.Owner
			entry["lockedUntil"] = lock.ExpiresAt.Format(time.RFC3339)
		}
		tabs = append(tabs, entry)
	}
	web.JSON(w, 200, map[string]any{"tabs": tabs})
}
