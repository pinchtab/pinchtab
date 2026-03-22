package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (h *Handlers) HandleStealthStatus(w http.ResponseWriter, r *http.Request) {
	status := h.Bridge.StealthStatus()
	if status == nil {
		httpx.JSON(w, 503, map[string]any{
			"status": "error",
			"reason": "stealth bundle unavailable",
		})
		return
	}
	if tabID := r.URL.Query().Get("tabId"); tabID != "" {
		if tracker, ok := h.Bridge.(interface{ FingerprintRotateActive(string) bool }); ok {
			status.TabOverrides["fingerprintRotateActive"] = tracker.FingerprintRotateActive(tabID)
		}
	}
	httpx.JSON(w, 200, status)
}
