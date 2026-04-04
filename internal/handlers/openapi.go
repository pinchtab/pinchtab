package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (h *Handlers) HandleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	security := h.endpointSecurityStates()
	httpx.JSON(w, 200, map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":   "Pinchtab API",
			"version": "0.7.x-local",
		},
		"x-pinchtab-security": security,
		"paths": map[string]any{
			"/health":          map[string]any{"get": map[string]any{"summary": "Health"}},
			"/browser/restart": map[string]any{"post": map[string]any{"summary": "Soft restart the browser process without restarting the bridge"}},
			"/tabs":            map[string]any{"get": map[string]any{"summary": "List tabs"}},
			"/metrics":         map[string]any{"get": map[string]any{"summary": "Runtime metrics"}},
			"/help":            map[string]any{"get": map[string]any{"summary": "Alias for /openapi.json"}},
			"/text":            map[string]any{"get": map[string]any{"summary": "Extract text", "parameters": []map[string]any{{"name": "maxChars", "in": "query", "schema": map[string]string{"type": "integer"}}, {"name": "format", "in": "query", "schema": map[string]string{"type": "string"}}}}},
			"/navigate":        map[string]any{"post": map[string]any{"summary": "Navigate"}, "get": map[string]any{"summary": "Navigate (query params)"}},
			"/nav":             map[string]any{"get": map[string]any{"summary": "Navigate alias"}},
			"/action":          map[string]any{"post": map[string]any{"summary": "Single action"}, "get": map[string]any{"summary": "Single action (query params)"}},
			"/actions":         map[string]any{"post": map[string]any{"summary": "Batch actions"}},
			"/snapshot":        map[string]any{"get": map[string]any{"summary": "Accessibility snapshot"}},
			"/evaluate": map[string]any{"post": map[string]any{
				"summary":            "Run JavaScript in the current tab",
				"description":        security["evaluate"].Message,
				"x-pinchtab-enabled": security["evaluate"].Enabled,
			}},
			"/tabs/{id}/evaluate": map[string]any{"post": map[string]any{
				"summary":            "Run JavaScript in a specific tab",
				"description":        security["evaluate"].Message,
				"x-pinchtab-enabled": security["evaluate"].Enabled,
			}},
			"/macro": map[string]any{"post": map[string]any{
				"summary":            "Macro action pipeline",
				"description":        security["macro"].Message,
				"x-pinchtab-enabled": security["macro"].Enabled,
			}},
			"/download": map[string]any{"get": map[string]any{
				"summary":            "Download a URL using the browser session",
				"description":        security["download"].Message,
				"x-pinchtab-enabled": security["download"].Enabled,
			}},
			"/tabs/{id}/download": map[string]any{"get": map[string]any{
				"summary":            "Download a URL with a specific tab context",
				"description":        security["download"].Message,
				"x-pinchtab-enabled": security["download"].Enabled,
			}},
			"/upload": map[string]any{"post": map[string]any{
				"summary":            "Set files on a file input",
				"description":        security["upload"].Message,
				"x-pinchtab-enabled": security["upload"].Enabled,
			}},
			"/tabs/{id}/upload": map[string]any{"post": map[string]any{
				"summary":            "Set files on a file input in a specific tab",
				"description":        security["upload"].Message,
				"x-pinchtab-enabled": security["upload"].Enabled,
			}},
			"/screencast": map[string]any{"get": map[string]any{
				"summary":            "Stream live tab frames",
				"description":        security["screencast"].Message,
				"x-pinchtab-enabled": security["screencast"].Enabled,
			}},
			"/screencast/tabs": map[string]any{"get": map[string]any{
				"summary":            "List tabs available for live capture",
				"description":        security["screencast"].Message,
				"x-pinchtab-enabled": security["screencast"].Enabled,
			}},
			"/storage": map[string]any{
				"get": map[string]any{
					"summary":            "Get localStorage/sessionStorage items (current origin only)",
					"description":        security["stateExport"].Message,
					"x-pinchtab-enabled": security["stateExport"].Enabled,
				},
				"post": map[string]any{
					"summary":            "Set a storage item",
					"description":        security["stateExport"].Message,
					"x-pinchtab-enabled": security["stateExport"].Enabled,
				},
				"delete": map[string]any{
					"summary":            "Delete storage items or clear storage",
					"description":        security["stateExport"].Message,
					"x-pinchtab-enabled": security["stateExport"].Enabled,
				},
			},
			"/state/list": map[string]any{"get": map[string]any{"summary": "List saved state files"}},
			// CapStateExport-gated endpoints
			"/state/show": map[string]any{"get": map[string]any{
				"summary":            "Show state file details",
				"description":        security["stateExport"].Message,
				"x-pinchtab-enabled": security["stateExport"].Enabled,
			}},
			"/state/save": map[string]any{"post": map[string]any{
				"summary":            "Save browser state (cookies, storage, metadata)",
				"description":        security["stateExport"].Message,
				"x-pinchtab-enabled": security["stateExport"].Enabled,
			}},
			"/state/load": map[string]any{"post": map[string]any{
				"summary":            "Load and restore browser state",
				"description":        security["stateExport"].Message,
				"x-pinchtab-enabled": security["stateExport"].Enabled,
			}},
			"/state": map[string]any{"delete": map[string]any{
				"summary":            "Delete a saved state file",
				"description":        security["stateExport"].Message,
				"x-pinchtab-enabled": security["stateExport"].Enabled,
			}},
			"/state/clean": map[string]any{"post": map[string]any{
				"summary":            "Clean old state files",
				"description":        security["stateExport"].Message,
				"x-pinchtab-enabled": security["stateExport"].Enabled,
			}},
		},
	})
}
