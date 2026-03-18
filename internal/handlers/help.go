package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (h *Handlers) HandleHelp(wr http.ResponseWriter, _ *http.Request) {
	security := h.endpointSecurityStates()
	httpx.JSON(wr, 200, map[string]any{
		"name": "pinchtab",
		"endpoints": map[string]any{
			"GET /health":              "health status",
			"GET /tabs":                "list tabs",
			"GET /metrics":             "runtime metrics",
			"GET /help":                "this help payload",
			"GET /openapi.json":        "lightweight machine-readable API schema",
			"GET /text":                "extract page text (supports mode=raw,maxChars=<int>,format=text)",
			"POST|GET /navigate":       "navigate tab (JSON body or query params)",
			"GET /nav":                 "alias for GET /navigate",
			"POST|GET /action":         "run a single action (JSON body or query params)",
			"POST /actions":            "run multiple actions",
			"GET /snapshot":            "accessibility snapshot",
			"POST /evaluate":           endpointStatusSummary(security["evaluate"], "run JavaScript in the current tab"),
			"POST /tabs/{id}/evaluate": endpointStatusSummary(security["evaluate"], "run JavaScript in a specific tab"),
			"POST /macro":              endpointStatusSummary(security["macro"], "run macro steps with single request"),
			"GET /download":            endpointStatusSummary(security["download"], "download a URL using the browser session"),
			"GET /tabs/{id}/download":  endpointStatusSummary(security["download"], "download a URL with a specific tab context"),
			"POST /upload":             endpointStatusSummary(security["upload"], "set files on a file input"),
			"POST /tabs/{id}/upload":   endpointStatusSummary(security["upload"], "set files on a file input in a specific tab"),
			"GET /screencast":          endpointStatusSummary(security["screencast"], "stream live tab frames"),
			"GET /screencast/tabs":     endpointStatusSummary(security["screencast"], "list tabs available for live capture"),
		},
		"security": security,
		"notes": []string{
			"Use Authorization: Bearer <token> when auth is enabled.",
			"Prefer /text with maxChars for token-efficient reads.",
		},
	})
}
