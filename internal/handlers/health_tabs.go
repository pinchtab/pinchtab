package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type tabHandoffReader interface {
	TabHandoffState(tabID string) (bridge.TabHandoffState, bool)
}

func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if h.Router != nil && h.Router.Mode() == engine.ModeLite {
		resp := map[string]any{
			"status": "ok",
			"engine": "lite",
		}
		if hasFailureDiagnostics() {
			resp["failures"] = FailureSnapshot()
		}
		if bridge.HasCrashDiagnostics() {
			resp["crashes"] = bridge.CrashSnapshot()
		}
		httpx.JSON(w, http.StatusOK, resp)
		return
	}

	// Guard against nil Bridge
	if h.Bridge == nil {
		httpx.JSON(w, 503, map[string]any{"status": "error", "reason": "bridge not initialized"})
		return
	}
	if draining, retryAfter := h.bridgeRestartStatus(); draining {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", seconds))
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"status": "draining", "retryAfterSeconds": seconds})
		return
	}

	// Ensure Chrome is initialized before checking health
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.JSON(w, 503, map[string]any{"status": "error", "reason": fmt.Sprintf("chrome initialization failed: %v", err)})
		return
	}

	targets, err := h.Bridge.ListTargets()
	if err != nil {
		httpx.JSON(w, 503, map[string]any{"status": "error", "reason": err.Error()})
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

	httpx.JSON(w, 200, resp)
}

func (h *Handlers) HandleEnsureChrome(w http.ResponseWriter, r *http.Request) {
	// Ensure Chrome is initialized for this instance
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization failed: %w", err))
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": "chrome_ready"})
}

func (h *Handlers) HandleBrowserRestart(w http.ResponseWriter, r *http.Request) {
	if h.Bridge == nil {
		httpx.Error(w, 503, fmt.Errorf("bridge not initialized"))
		return
	}
	if err := h.Bridge.RestartBrowser(h.Config); err != nil {
		httpx.Error(w, 500, fmt.Errorf("browser restart failed: %w", err))
		return
	}
	httpx.JSON(w, 200, map[string]string{"status": "browser_restarted"})
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

	httpx.JSON(w, 200, result)
}

func (h *Handlers) HandleTabMetrics(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("missing tab id"))
		return
	}

	if h.Bridge == nil {
		httpx.Error(w, 503, fmt.Errorf("bridge not initialized"))
		return
	}

	if _, _, err := h.Bridge.TabContext(tabID); err != nil {
		httpx.Error(w, http.StatusNotFound, fmt.Errorf("tab not found"))
		return
	}

	mem, err := h.Bridge.GetMemoryMetrics(tabID)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("failed to get metrics: %w", err))
		return
	}

	httpx.JSON(w, 200, mem)
}

func (h *Handlers) HandleTabs(w http.ResponseWriter, r *http.Request) {
	// Guard against nil Bridge
	if h.Bridge == nil {
		httpx.Error(w, 503, fmt.Errorf("bridge not initialized"))
		return
	}
	if draining, retryAfter := h.bridgeRestartStatus(); draining {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", seconds))
		httpx.ErrorCode(w, http.StatusServiceUnavailable, "browser_draining", bridge.ErrBrowserDraining.Error(), true, map[string]any{"retryAfterSeconds": seconds})
		return
	}

	targets, err := h.Bridge.ListTargets()
	if err != nil {
		httpx.Error(w, 503, err)
		return
	}

	currentTabID := ""
	if _, resolvedID, err := h.Bridge.TabContext(""); err == nil {
		currentTabID = resolvedID
	}

	tabs := make([]map[string]any, 0, len(targets))
	appendTab := func(t *target.Info) {
		// Skip the initial about:blank tab that Chrome creates on launch
		if bridge.IsTransientURL(t.URL, h.Config.Port) {
			return
		}
		tabID := string(t.TargetID)
		entry := map[string]any{
			"id":    tabID,
			"url":   t.URL,
			"title": t.Title,
			"type":  t.Type,
		}
		if hr, ok := h.Bridge.(tabHandoffReader); ok {
			if hs, ok := hr.TabHandoffState(tabID); ok {
				entry["status"] = hs.Status
				entry["handoffReason"] = hs.Reason
				entry["pausedAt"] = hs.PausedAt.Format(time.RFC3339)
			} else {
				entry["status"] = "active"
			}
		}
		if lock := h.Bridge.TabLockInfo(tabID); lock != nil {
			entry["owner"] = lock.Owner
			entry["lockedUntil"] = lock.ExpiresAt.Format(time.RFC3339)
		}
		tabs = append(tabs, entry)
	}

	// First pass: add the current focused tab
	for _, t := range targets {
		if string(t.TargetID) == currentTabID {
			appendTab(t)
		}
	}
	// Second pass: add all other tabs
	for _, t := range targets {
		if string(t.TargetID) == currentTabID {
			continue
		}
		appendTab(t)
	}
	httpx.JSON(w, 200, map[string]any{"tabs": tabs})
}
