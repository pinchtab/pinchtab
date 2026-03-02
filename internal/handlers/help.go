package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (h *Handlers) HandleHelp(wr http.ResponseWriter, _ *http.Request) {
	web.JSON(wr, 200, map[string]any{
		"name": "pinchtab",
		"endpoints": map[string]any{
			"GET /health":        "health status",
			"GET /tabs":          "list tabs",
			"GET /metrics":       "runtime metrics",
			"GET /help":          "this help payload",
			"GET /openapi.json":  "lightweight machine-readable API schema",
			"GET /text":          "extract page text (supports mode=raw,maxChars=<int>,format=text)",
			"POST|GET /navigate": "navigate tab (JSON body or query params)",
			"GET /nav":           "alias for GET /navigate",
			"POST|GET /action":   "run a single action (JSON body or query params)",
			"POST /actions":      "run multiple actions",
			"POST /macro":        "run macro steps with single request",
			"GET /snapshot":      "accessibility snapshot",
		},
		"notes": []string{
			"Use Authorization: Bearer <token> when auth is enabled.",
			"Prefer /text with maxChars for token-efficient reads.",
		},
	})
}
