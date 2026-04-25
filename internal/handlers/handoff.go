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
	SetTabHandoff(tabID, reason string, timeout time.Duration) error
	ResumeTabHandoff(tabID string) error
	TabHandoffState(tabID string) (bridge.TabHandoffState, bool)
}

func (h *Handlers) handoffController() (tabHandoffController, bool) {
	ctrl, ok := h.Bridge.(tabHandoffController)
	return ctrl, ok
}

// handoffHintMessage is included in error responses and dashboard events when
// the agent must yield control to a human operator.
const handoffHintMessage = "return control to the user and ask them to manually solve the challenge in the browser window, then call POST /tabs/{id}/resume to continue"

// handoffErrorDetails builds the details payload attached to 409 responses
// when an action hits a tab that is paused for handoff. Always includes the
// agent hint; when known, also includes the current reason and pausedAt.
func (h *Handlers) handoffErrorDetails(tabID string) map[string]any {
	details := map[string]any{"hint": handoffHintMessage}
	if ctrl, ok := h.handoffController(); ok {
		if state, exists := ctrl.TabHandoffState(tabID); exists {
			if state.Reason != "" {
				details["reason"] = state.Reason
			}
			if !state.PausedAt.IsZero() {
				details["pausedAt"] = state.PausedAt.Format(time.RFC3339)
			}
		}
	}
	return details
}

// pauseTabForHandoff marks a tab as paused for human handoff and broadcasts
// a dashboard event. source identifies what triggered the handoff
// (e.g. "autosolver", "manual"). Returns the applied reason.
func (h *Handlers) pauseTabForHandoff(tabID, reason, source string, timeout time.Duration) (string, error) {
	ctrl, ok := h.handoffController()
	if !ok {
		return "", fmt.Errorf("bridge does not support handoff state")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual_handoff"
	}
	if err := ctrl.SetTabHandoff(tabID, reason, timeout); err != nil {
		return reason, err
	}
	if h.Dashboard != nil {
		payload := map[string]any{
			"tabId":       tabID,
			"status":      "paused_handoff",
			"reason":      reason,
			"source":      source,
			"hint":        handoffHintMessage,
			"requestedAt": time.Now().UTC().Format(time.RFC3339),
		}
		if timeout > 0 {
			payload["timeoutMs"] = int(timeout / time.Millisecond)
		}
		h.Dashboard.BroadcastSystemEvent(dashboard.SystemEvent{
			Type:     "tab.handoff",
			Instance: payload,
		})
	}
	return reason, nil
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

	if _, ok := h.handoffController(); !ok {
		httpx.ErrorCode(w, 501, "handoff_not_supported", "bridge does not support handoff state", false, nil)
		return
	}
	if req.TimeoutMs < 0 {
		httpx.Error(w, 400, fmt.Errorf("timeoutMs must be >= 0"))
		return
	}
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	reason, err := h.pauseTabForHandoff(resolvedTabID, req.Reason, "manual", timeout)
	if err != nil {
		httpx.ErrorCode(w, 500, "handoff_failed", err.Error(), false, nil)
		return
	}

	h.recordActivity(r, activity.Update{Action: "handoff", TabID: resolvedTabID})

	resp := map[string]any{
		"tabId":     resolvedTabID,
		"status":    "paused_handoff",
		"reason":    reason,
		"timeoutMs": req.TimeoutMs,
		"hint":      handoffHintMessage,
	}
	if timeout > 0 {
		resp["expiresAt"] = time.Now().UTC().Add(timeout).Format(time.RFC3339)
	}
	httpx.JSON(w, 200, resp)
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
		resp := map[string]any{
			"tabId":         resolvedTabID,
			"status":        state.Status,
			"reason":        state.Reason,
			"pausedAt":      state.PausedAt.Format(time.RFC3339),
			"lastUpdatedAt": state.LastUpdatedAt.Format(time.RFC3339),
		}
		if !state.ExpiresAt.IsZero() {
			resp["expiresAt"] = state.ExpiresAt.Format(time.RFC3339)
			resp["timeoutMs"] = int(state.ExpiresAt.Sub(state.PausedAt).Milliseconds())
		}
		httpx.JSON(w, 200, resp)
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"tabId":  resolvedTabID,
		"status": "active",
	})
}
