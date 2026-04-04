package strategy

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// capabilityEnabled maps a routes.Capability to the orchestrator check.
func capabilityEnabled(orch *orchestrator.Orchestrator, cap routes.Capability) bool {
	switch cap {
	case routes.CapEvaluate:
		return orch.AllowsEvaluate()
	case routes.CapMacro:
		return orch.AllowsMacro()
	case routes.CapScreencast:
		return orch.AllowsScreencast()
	case routes.CapDownload:
		return orch.AllowsDownload()
	case routes.CapUpload:
		return orch.AllowsUpload()
	default:
		return false
	}
}

// capabilitySetting returns the config key and disabled code for a capability.
func capabilitySetting(cap routes.Capability) (feature, setting, code string) {
	switch cap {
	case routes.CapEvaluate:
		return "evaluate", "security.allowEvaluate", "evaluate_disabled"
	case routes.CapMacro:
		return "macro", "security.allowMacro", "macro_disabled"
	case routes.CapScreencast:
		return "screencast", "security.allowScreencast", "screencast_disabled"
	case routes.CapDownload:
		return "download", "security.allowDownload", "download_disabled"
	case routes.CapUpload:
		return "upload", "security.allowUpload", "upload_disabled"
	default:
		return string(cap), "security.allow" + string(cap), string(cap) + "_disabled"
	}
}

// RegisterShorthandRoutes registers all shorthand proxy routes on the mux,
// binding them to the given handler. It also registers capability-gated
// routes (evaluate, download, upload, screencast, macro) using the
// orchestrator's security settings.
//
// Routes are sourced from the shared routes.Core() catalogue.
func RegisterShorthandRoutes(mux *http.ServeMux, orch *orchestrator.Orchestrator, handler http.HandlerFunc) {
	for _, route := range routes.ShorthandRoutes() {
		mux.HandleFunc(route, handler)
	}

	for cap, eps := range routes.CapabilityEndpoints() {
		enabled := capabilityEnabled(orch, cap)
		feature, setting, code := capabilitySetting(cap)
		for _, ep := range eps {
			RegisterCapabilityRoute(mux, ep.Route(), enabled, feature, setting, code, handler)
		}
	}
}
