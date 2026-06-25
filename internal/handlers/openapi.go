package handlers

import (
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

func (h *Handlers) HandleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	security := h.endpointSecurityStates()

	paths := map[string]map[string]any{}
	addOp := func(path, method string, op map[string]any) {
		m := paths[path]
		if m == nil {
			m = map[string]any{}
			paths[path] = m
		}
		m[strings.ToLower(method)] = op
	}

	operationFor := func(ep routes.Endpoint) map[string]any {
		op := map[string]any{"summary": ep.Summary}
		if ep.Capability != routes.CapNone {
			if st, ok := security[string(ep.Capability)]; ok {
				op["description"] = st.Message
				op["x-pinchtab-enabled"] = st.Enabled
			}
		}
		return op
	}

	// Baseline: every catalog route. Root entry unless the endpoint is registered
	// only in its /tabs/{id}/... form, plus the tab-scoped variant where applicable.
	for _, ep := range routes.Core() {
		if !tabOnlyRoutes[ep.Route()] {
			addOp(ep.Path, ep.Method, operationFor(ep))
		}
		if ep.TabScoped {
			addOp("/tabs/{id}"+ep.Path, ep.Method, operationFor(ep))
		}
	}

	// Non-catalog meta/docs/alias routes (registered outside the catalog loop).
	// Management routes (/ensure-*, /shutdown, /openapi.json) stay undocumented.
	addOp("/health", "GET", map[string]any{"summary": "Health"})
	addOp("/browser/restart", "POST", map[string]any{"summary": "Soft restart the browser process without restarting the bridge"})
	addOp("/tabs", "GET", map[string]any{"summary": "List tabs"})
	addOp("/help", "GET", map[string]any{"summary": "Alias for /openapi.json"})
	addOp("/navigate", "GET", map[string]any{"summary": "Navigate (query params)"})
	addOp("/action", "GET", map[string]any{"summary": "Single action (query params)"})

	// Per-operation extras layered onto the generated ops.
	evaluateRequestBody := map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"tabId": map[string]any{
							"type":        "string",
							"description": "Optional tab ID for top-level /evaluate requests",
						},
						"expression": map[string]any{
							"type":        "string",
							"description": "JavaScript expression to evaluate",
						},
						"awaitPromise": map[string]any{
							"type":        "boolean",
							"description": "Wait for a returned promise to resolve before returning the result",
						},
					},
					"required": []string{"expression"},
				},
			},
		},
	}
	for _, p := range []string{"/evaluate", "/tabs/{id}/evaluate"} {
		if op, ok := paths[p]["post"].(map[string]any); ok {
			op["requestBody"] = evaluateRequestBody
		}
	}
	if op, ok := paths["/text"]["get"].(map[string]any); ok {
		op["parameters"] = []map[string]any{
			{"name": "maxChars", "in": "query", "schema": map[string]string{"type": "integer"}},
			{"name": "format", "in": "query", "schema": map[string]string{"type": "string"}},
			{"name": "mode", "in": "query", "schema": map[string]string{"type": "string"}},
			{"name": "frameId", "in": "query", "schema": map[string]string{"type": "string"}},
		}
	}

	httpx.JSON(w, 200, map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":   "Pinchtab API",
			"version": "0.7.x-local",
		},
		"x-pinchtab-security": security,
		"paths":               paths,
	})
}
