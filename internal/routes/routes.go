// Package routes defines the canonical API endpoint catalogue shared
// across bridge, orchestrator, and strategy layers. Adding a new
// endpoint here automatically propagates it to all layers and to
// the generated /openapi.json response.
package routes

import "fmt"

// Capability gates an endpoint behind a security config flag.
type Capability string

const (
	CapNone        Capability = ""
	CapEvaluate    Capability = "evaluate"
	CapMacro       Capability = "macro"
	CapScreencast  Capability = "screencast"
	CapDownload    Capability = "download"
	CapUpload      Capability = "upload"
	CapStateExport Capability = "stateExport"
)

// Endpoint describes a single API route.
type Endpoint struct {
	Method     string     // HTTP method: "GET", "POST"
	Path       string     // Shorthand path: "/snapshot", "/navigate"
	Summary    string     // Human-readable description
	Capability Capability // "" = always enabled, otherwise capability-gated
	TabScoped  bool       // true = auto-generates /tabs/{id}/... variant
}

// Route returns the "METHOD /path" string used for mux registration.
func (e Endpoint) Route() string {
	return e.Method + " " + e.Path
}

// TabRoute returns the tab-scoped variant: "METHOD /tabs/{id}/path".
// Panics if TabScoped is false.
func (e Endpoint) TabRoute() string {
	if !e.TabScoped {
		panic(fmt.Sprintf("endpoint %s %s is not tab-scoped", e.Method, e.Path))
	}
	return e.Method + " /tabs/{id}" + e.Path
}

// coreEndpoints is the canonical list of API endpoints.
var coreEndpoints = []Endpoint{
	// Navigation
	{"POST", "/navigate", "Navigate to URL", CapNone, true},
	{"POST", "/back", "Go back", CapNone, true},
	{"POST", "/forward", "Go forward", CapNone, true},
	{"POST", "/reload", "Reload page", CapNone, true},

	// Content extraction
	{"GET", "/snapshot", "Accessibility snapshot", CapNone, true},
	{"GET", "/frame", "Get current frame scope", CapNone, true},
	{"POST", "/frame", "Set current frame scope", CapNone, true},
	{"GET", "/screenshot", "Page screenshot", CapNone, true},
	{"GET", "/text", "Extract page text", CapNone, true},
	{"GET", "/pdf", "Export as PDF (GET)", CapNone, true},
	{"POST", "/pdf", "Export as PDF (POST)", CapNone, true},

	// Actions
	{"POST", "/action", "Single action", CapNone, true},
	{"POST", "/actions", "Batch actions", CapNone, true},
	{"POST", "/dialog", "Handle dialog", CapNone, true},
	{"POST", "/wait", "Wait for condition", CapNone, true},
	{"POST", "/find", "Find elements", CapNone, true},

	// Tab management
	{"POST", "/tab", "Open or switch tab", CapNone, false},
	{"POST", "/lock", "Lock tab", CapNone, true},
	{"POST", "/unlock", "Unlock tab", CapNone, true},

	// Human handoff
	{"POST", "/handoff", "Pause tab for human handoff", CapNone, true},
	{"POST", "/resume", "Resume paused tab", CapNone, true},
	{"GET", "/handoff", "Get handoff status", CapNone, true},

	// Cookies
	{"GET", "/cookies", "Get cookies", CapNone, true},
	{"POST", "/cookies", "Set cookies", CapNone, true},

	// Metrics
	{"GET", "/metrics", "Runtime metrics", CapNone, true},

	// Network
	{"GET", "/network", "Network log", CapNone, true},
	{"GET", "/network/stream", "Network SSE stream", CapNone, true},
	{"GET", "/network/export", "Export HAR", CapNone, true},
	{"GET", "/network/export/stream", "Export HAR stream", CapNone, true},
	{"GET", "/network/{requestId}", "Single network request", CapNone, true},
	{"POST", "/network/clear", "Clear network log", CapNone, false},

	// Console & errors
	{"GET", "/console", "Console logs", CapNone, false},
	{"POST", "/console/clear", "Clear console logs", CapNone, false},
	{"GET", "/errors", "Error logs", CapNone, false},
	{"POST", "/errors/clear", "Clear error logs", CapNone, false},

	// Clipboard
	{"GET", "/clipboard/read", "Read clipboard", CapNone, false},
	{"POST", "/clipboard/write", "Write clipboard", CapNone, false},
	{"POST", "/clipboard/copy", "Copy to clipboard", CapNone, false},
	{"GET", "/clipboard/paste", "Paste from clipboard", CapNone, false},

	// Stealth & fingerprint
	{"GET", "/stealth/status", "Stealth configuration status", CapNone, false},
	{"POST", "/fingerprint/rotate", "Rotate browser fingerprint", CapNone, false},

	// Solvers
	{"GET", "/solvers", "List available solvers", CapNone, false},
	{"GET", "/config/autosolver", "Get autosolver runtime config", CapNone, false},
	{"POST", "/solve", "Run default solver", CapNone, true},
	{"POST", "/solve/{name}", "Run named solver", CapNone, true},

	// Cache
	{"POST", "/cache/clear", "Clear browser cache", CapNone, false},
	{"GET", "/cache/status", "Cache status", CapNone, false},

	// Storage operations are gated under stateExport because they access/mutate sensitive client-side state.
	{"POST", "/storage", "Set storage item", CapStateExport, true},
	{"DELETE", "/storage", "Delete storage items", CapStateExport, true},

	// State management
	{"GET", "/state/list", "List saved states", CapStateExport, false},

	// Capability-gated
	{"POST", "/evaluate", "Run JavaScript in page", CapEvaluate, true},
	{"POST", "/macro", "Macro action pipeline", CapMacro, false},
	{"GET", "/download", "Download URL via browser session", CapDownload, true},
	{"POST", "/upload", "Upload file to file input", CapUpload, true},
	{"GET", "/screencast", "Live tab frame stream", CapScreencast, false},
	{"GET", "/screencast/tabs", "List tabs available for screencast", CapScreencast, false},
	// CapStateExport gates all sensitive state I/O: reading, writing, injection, and deletion.
	{"GET", "/storage", "Get storage items (current origin)", CapStateExport, true},
	{"GET", "/state/show", "Show state file details", CapStateExport, false},
	{"POST", "/state/save", "Save browser state", CapStateExport, false},
	{"POST", "/state/load", "Load and restore browser state", CapStateExport, false},
	{"DELETE", "/state", "Delete saved state file", CapStateExport, false},
	{"POST", "/state/clean", "Clean old state files", CapStateExport, false},
}

// Core returns a copy of the canonical endpoint list.
func Core() []Endpoint {
	out := make([]Endpoint, len(coreEndpoints))
	copy(out, coreEndpoints)
	return out
}

// ShorthandRoutes returns all non-capability-gated shorthand routes
// as "METHOD /path" strings, suitable for mux registration.
func ShorthandRoutes() []string {
	var routes []string
	for _, ep := range coreEndpoints {
		if ep.Capability == CapNone {
			routes = append(routes, ep.Route())
		}
	}
	return routes
}

// CapabilityEndpoints returns endpoints grouped by their capability gate.
func CapabilityEndpoints() map[Capability][]Endpoint {
	m := make(map[Capability][]Endpoint)
	for _, ep := range coreEndpoints {
		if ep.Capability != CapNone {
			m[ep.Capability] = append(m[ep.Capability], ep)
		}
	}
	return m
}

// TabScopedRoutes returns "METHOD /tabs/{id}/path" for all tab-scoped
// endpoints that are NOT capability-gated (those need separate handling).
func TabScopedRoutes() []string {
	var routes []string
	for _, ep := range coreEndpoints {
		if ep.TabScoped && ep.Capability == CapNone {
			routes = append(routes, ep.TabRoute())
		}
	}
	return routes
}

// TabScopedCapabilityRoutes returns tab-scoped capability-gated endpoints.
func TabScopedCapabilityRoutes() map[Capability][]Endpoint {
	m := make(map[Capability][]Endpoint)
	for _, ep := range coreEndpoints {
		if ep.TabScoped && ep.Capability != CapNone {
			m[ep.Capability] = append(m[ep.Capability], ep)
		}
	}
	return m
}
