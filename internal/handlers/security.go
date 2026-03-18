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

func (h *Handlers) uploadEnabled() bool {
	return h != nil && h.Config != nil && h.Config.AllowUpload
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
			Paths:   []string{"GET /screencast", "GET /screencast/tabs", "GET /instances/{id}/screencast", "GET /instances/{id}/proxy/screencast"},
		},
		"download": {
			Enabled: h.downloadEnabled(),
			Setting: "security.allowDownload",
			Message: httpx.DisabledEndpointMessage("download", "security.allowDownload"),
			Paths:   []string{"GET /download", "GET /tabs/{id}/download"},
		},
		"upload": {
			Enabled: h.uploadEnabled(),
			Setting: "security.allowUpload",
			Message: httpx.DisabledEndpointMessage("upload", "security.allowUpload"),
			Paths:   []string{"POST /upload", "POST /tabs/{id}/upload"},
		},
	}
}

func endpointStatusSummary(state endpointSecurityState, summary string) string {
	if state.Enabled {
		return summary + " (enabled)"
	}
	return summary + " (locked by " + state.Setting + ")"
}
