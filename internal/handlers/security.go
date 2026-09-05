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
		httpx.DisabledEndpointDetails(meta.Setting))
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

func (h *Handlers) allows(cap routes.Capability) bool {
	return h != nil && h.Config.CapabilityEnabled(cap)
}

var instanceScopedCapabilityPaths = map[routes.Capability][]string{
	routes.CapScreencast: {"GET /instances/{id}/screencast", "GET /instances/{id}/proxy/screencast"},
}

func capabilityPaths(cap routes.Capability) []string {
	var paths []string
	for _, ep := range routes.CapabilityEndpoints()[cap] {
		paths = append(paths, ep.Route())
		if ep.TabScoped {
			paths = append(paths, ep.TabRoute())
		}
	}
	return append(paths, instanceScopedCapabilityPaths[cap]...)
}

func (h *Handlers) endpointSecurityStates() map[string]endpointSecurityState {
	states := make(map[string]endpointSecurityState)
	for _, cap := range routes.Capabilities() {
		states[string(cap)] = capState(cap, h.allows(cap), capabilityPaths(cap))
	}
	states["clipboard"] = endpointSecurityState{
		Enabled: h.clipboardEnabled(),
		Setting: clipboardSetting,
		Message: httpx.DisabledEndpointMessage("clipboard", clipboardSetting),
		Paths:   []string{"GET /clipboard/read", "POST /clipboard/write", "POST /clipboard/copy", "GET /clipboard/paste"},
	}
	return states
}
