package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type tabHandoffController interface {
	SetTabHandoff(tabID, reason string) error
	ResumeTabHandoff(tabID string) error
	TabHandoffState(tabID string) (bridge.TabHandoffState, bool)
}

func (h *Handlers) handoffController() (tabHandoffController, bool) {
	ctrl, ok := h.Bridge.(tabHandoffController)
	return ctrl, ok
}

func (h *Handlers) HandleTabHandoff(w http.ResponseWriter, r *http.Request) {
	tabID := strings.TrimSpace(r.PathValue("id"))
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	var req struct {
		Reason    string `json:"reason"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	owner := resolveOwner(r, "")
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		httpx.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	ctrl, ok := h.handoffController()
	if !ok {
		httpx.ErrorCode(w, 501, "handoff_not_supported", "bridge does not support handoff state", false, nil)
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual_handoff"
	}
	if err := ctrl.SetTabHandoff(resolvedTabID, reason); err != nil {
		httpx.ErrorCode(w, 500, "handoff_failed", err.Error(), false, nil)
		return
	}

	h.recordActivity(r, activity.Update{Action: "handoff", TabID: resolvedTabID})
	if h.Dashboard != nil {
		h.Dashboard.BroadcastSystemEvent(dashboard.SystemEvent{
			Type: "tab.handoff",
			Instance: map[string]any{
				"tabId":       resolvedTabID,
				"status":      "paused_handoff",
				"reason":      reason,
				"timeoutMs":   req.TimeoutMs,
				"requestedAt": time.Now().UTC().Format(time.RFC3339),
			},
		})
	}

	httpx.JSON(w, 200, map[string]any{
		"tabId":     resolvedTabID,
		"status":    "paused_handoff",
		"reason":    reason,
		"timeoutMs": req.TimeoutMs,
	})
}

func (h *Handlers) HandleTabResume(w http.ResponseWriter, r *http.Request) {
	tabID := strings.TrimSpace(r.PathValue("id"))
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	var req struct {
		Status string         `json:"status"`
		Data   map[string]any `json:"resolvedData"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	owner := resolveOwner(r, "")
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		httpx.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	ctrl, ok := h.handoffController()
	if !ok {
		httpx.ErrorCode(w, 501, "handoff_not_supported", "bridge does not support handoff state", false, nil)
		return
	}
	if err := ctrl.ResumeTabHandoff(resolvedTabID); err != nil {
		httpx.ErrorCode(w, 500, "resume_failed", err.Error(), false, nil)
		return
	}

	h.recordActivity(r, activity.Update{Action: "resume", TabID: resolvedTabID})
	if h.Dashboard != nil {
		h.Dashboard.BroadcastSystemEvent(dashboard.SystemEvent{
			Type: "tab.resume",
			Instance: map[string]any{
				"tabId":        resolvedTabID,
				"status":       strings.TrimSpace(req.Status),
				"resolvedData": req.Data,
				"resumedAt":    time.Now().UTC().Format(time.RFC3339),
			},
		})
	}

	httpx.JSON(w, 200, map[string]any{
		"tabId":        resolvedTabID,
		"status":       "active",
		"resumeStatus": strings.TrimSpace(req.Status),
		"resolvedData": req.Data,
	})
}

func (h *Handlers) HandleTabHandoffStatus(w http.ResponseWriter, r *http.Request) {
	tabID := strings.TrimSpace(r.PathValue("id"))
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	_, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	ctrl, ok := h.handoffController()
	if !ok {
		httpx.ErrorCode(w, 501, "handoff_not_supported", "bridge does not support handoff state", false, nil)
		return
	}
	if state, ok := ctrl.TabHandoffState(resolvedTabID); ok {
		httpx.JSON(w, 200, map[string]any{
			"tabId":         resolvedTabID,
			"status":        state.Status,
			"reason":        state.Reason,
			"pausedAt":      state.PausedAt.Format(time.RFC3339),
			"lastUpdatedAt": state.LastUpdatedAt.Format(time.RFC3339),
		})
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"tabId":  resolvedTabID,
		"status": "active",
	})
}
