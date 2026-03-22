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
	httpx.JSON(w, 200, status)
}
