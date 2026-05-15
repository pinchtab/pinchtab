package handlers

import "github.com/pinchtab/pinchtab/internal/httpx"

type endpointSecurityState struct {
	Enabled bool     `json:"enabled"`
	Setting string   `json:"setting"`
	Message string   `json:"message"`
	Paths   []string `json:"paths"`
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
		"evaluate": {
			Enabled: h.evaluateEnabled(),
			Setting: "security.allowEvaluate",
			Message: httpx.DisabledEndpointMessage("evaluate", "security.allowEvaluate"),
			Paths:   []string{"POST /evaluate", "POST /tabs/{id}/evaluate"},
		},
		"macro": {
			Enabled: h.macroEnabled(),
			Setting: "security.allowMacro",
			Message: httpx.DisabledEndpointMessage("macro", "security.allowMacro"),
			Paths:   []string{"POST /macro"},
		},
		"screencast": {
			Enabled: h.screencastEnabled(),
			Setting: "security.allowScreencast",
			Message: httpx.DisabledEndpointMessage("screencast", "security.allowScreencast"),
			Paths:   []string{"GET /screencast", "GET /screencast/tabs", "POST /record/start", "POST /record/stop", "GET /record/status", "GET /instances/{id}/screencast", "GET /instances/{id}/proxy/screencast"},
		},
		"download": {
			Enabled: h.downloadEnabled(),
			Setting: "security.allowDownload",
			Message: httpx.DisabledEndpointMessage("download", "security.allowDownload"),
			Paths:   []string{"GET /download", "GET /tabs/{id}/download"},
		},
		"cookies": {
			Enabled: h.cookiesEnabled(),
			Setting: "security.allowCookies",
			Message: httpx.DisabledEndpointMessage("cookies", "security.allowCookies"),
			Paths:   []string{"GET /cookies", "POST /cookies", "DELETE /cookies", "GET /tabs/{id}/cookies", "POST /tabs/{id}/cookies", "DELETE /tabs/{id}/cookies"},
		},
		"upload": {
			Enabled: h.uploadEnabled(),
			Setting: "security.allowUpload",
			Message: httpx.DisabledEndpointMessage("upload", "security.allowUpload"),
			Paths:   []string{"POST /upload", "POST /tabs/{id}/upload"},
		},
		"clipboard": {
			Enabled: h.clipboardEnabled(),
			Setting: "security.allowClipboard",
			Message: httpx.DisabledEndpointMessage("clipboard", "security.allowClipboard"),
			Paths:   []string{"GET /clipboard/read", "POST /clipboard/write", "POST /clipboard/copy", "GET /clipboard/paste"},
		},
		"stateExport": {
			Enabled: h.stateExportEnabled(),
			Setting: "security.allowStateExport",
			Message: httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"),
			Paths: []string{
				"GET /storage", "POST /storage", "DELETE /storage",
				"GET /tabs/{id}/storage", "POST /tabs/{id}/storage", "DELETE /tabs/{id}/storage",
				"GET /state/list", "GET /state/show", "POST /state/save",
				"POST /state/load", "DELETE /state", "POST /state/clean",
			},
		},
		"networkIntercept": {
			Enabled: h.networkInterceptEnabled(),
			Setting: "security.allowNetworkIntercept",
			Message: httpx.DisabledEndpointMessage("networkIntercept", "security.allowNetworkIntercept"),
			Paths: []string{
				"GET /network/route", "POST /network/route", "DELETE /network/route",
				"GET /tabs/{id}/network/route", "POST /tabs/{id}/network/route", "DELETE /tabs/{id}/network/route",
			},
		},
	}
}
