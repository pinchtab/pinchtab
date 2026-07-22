package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

type endpointSecurityState struct {
	Enabled bool     `json:"enabled"`
	Setting string   `json:"setting"`
	Message string   `json:"message"`
	Paths   []string `json:"paths"`
}

// writeCapabilityDisabled emits the standard 403 for a capability-gated endpoint
// using the centralized routes.Meta metadata, so the disabled error code,
// setting path, and message are defined once rather than restated at each gate.
func (h *Handlers) writeCapabilityDisabled(w http.ResponseWriter, cap routes.Capability) {
	meta, _ := routes.Meta(cap)
	httpx.ErrorCode(w, http.StatusForbidden, meta.DisabledCode,
		httpx.DisabledEndpointMessage(meta.Label, meta.Setting), false,
		map[string]any{"setting": meta.Setting})
}

// capState builds the /security projection for a capability-gated endpoint
// family, sourcing the setting path and message from the centralized metadata.
func capState(cap routes.Capability, enabled bool, paths []string) endpointSecurityState {
	meta, _ := routes.Meta(cap)
	return endpointSecurityState{
		Enabled: enabled,
		Setting: meta.Setting,
		Message: httpx.DisabledEndpointMessage(meta.Label, meta.Setting),
		Paths:   paths,
	}
}

func (h *Handlers) macroEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowMacro
}

func (h *Handlers) screencastEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowScreencast
}

func (h *Handlers) downloadEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowDownload
}

func (h *Handlers) cookiesEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowCookies
}

func (h *Handlers) uploadEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowUpload
}

func (h *Handlers) networkInterceptEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowNetworkIntercept
}

func (h *Handlers) endpointSecurityStates() map[string]endpointSecurityState {
	return map[string]endpointSecurityState{
		"evaluate": capState(routes.CapEvaluate, h.evaluateEnabled(),
			[]string{"POST /evaluate", "POST /tabs/{id}/evaluate"}),
		"macro": capState(routes.CapMacro, h.macroEnabled(),
			[]string{"POST /macro"}),
		"screencast": capState(routes.CapScreencast, h.screencastEnabled(),
			[]string{"GET /screencast", "GET /screencast/tabs", "POST /record/start", "POST /record/stop", "GET /record/status", "GET /instances/{id}/screencast", "GET /instances/{id}/proxy/screencast"}),
		"download": capState(routes.CapDownload, h.downloadEnabled(),
			[]string{"GET /download", "GET /tabs/{id}/download"}),
		"cookies": capState(routes.CapCookies, h.cookiesEnabled(),
			[]string{"GET /cookies", "POST /cookies", "DELETE /cookies", "GET /tabs/{id}/cookies", "POST /tabs/{id}/cookies", "DELETE /tabs/{id}/cookies"}),
		"upload": capState(routes.CapUpload, h.uploadEnabled(),
			[]string{"POST /upload", "POST /tabs/{id}/upload"}),
		// clipboard has no capability gate in the route catalog, so its metadata stays local.
		"clipboard": {
			Enabled: h.clipboardEnabled(),
			Setting: "security.allowClipboard",
			Message: httpx.DisabledEndpointMessage("clipboard", "security.allowClipboard"),
			Paths:   []string{"GET /clipboard/read", "POST /clipboard/write", "POST /clipboard/copy", "GET /clipboard/paste"},
		},
		"stateExport": capState(routes.CapStateExport, h.stateExportEnabled(),
			[]string{
				"GET /storage", "POST /storage", "DELETE /storage",
				"GET /tabs/{id}/storage", "POST /tabs/{id}/storage", "DELETE /tabs/{id}/storage",
				"GET /state", "GET /state/list", "GET /state/show", "POST /state/save",
				"POST /state/load", "DELETE /state", "POST /state/clean",
			}),
		"networkIntercept": capState(routes.CapNetworkIntercept, h.networkInterceptEnabled(),
			[]string{
				"GET /network/{requestId}", "GET /tabs/{id}/network/{requestId}", "POST /network/clear",
				"GET /network/route", "POST /network/route", "DELETE /network/route",
				"GET /tabs/{id}/network/route", "POST /tabs/{id}/network/route", "DELETE /tabs/{id}/network/route",
			}),
	}
}
