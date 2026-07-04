// Package routes defines the canonical API endpoint catalogue shared
// across bridge, orchestrator, and strategy layers. Adding a new
// endpoint here automatically propagates it to all layers and to
// the generated /openapi.json response.
package routes

import "fmt"

// Capability gates an endpoint behind a security config flag.
type Capability string

const (
	CapNone             Capability = ""
	CapEvaluate         Capability = "evaluate"
	CapMacro            Capability = "macro"
	CapScreencast       Capability = "screencast"
	CapDownload         Capability = "download"
	CapCookies          Capability = "cookies"
	CapUpload           Capability = "upload"
	CapStateExport      Capability = "stateExport"
	CapNetworkIntercept Capability = "networkIntercept"
)

// CapabilityMeta is the single source of truth for a capability gate's
// HTTP-facing metadata: the human label used in messages, the config path that
// enables it, and the stable error code returned when it is disabled. These
// strings are part of the API contract (clients string-match the code), so they
// are declared explicitly here rather than derived, and consumed by both the
// bridge handlers and the orchestrator instead of being restated at each gate.
type CapabilityMeta struct {
	Capability   Capability
	Label        string // message label, e.g. "evaluate"
	Setting      string // config path, e.g. "security.allowEvaluate"
	DisabledCode string // error code when disabled, e.g. "evaluate_disabled"
}

var capabilityMeta = map[Capability]CapabilityMeta{
	CapEvaluate:         {CapEvaluate, "evaluate", "security.allowEvaluate", "evaluate_disabled"},
	CapMacro:            {CapMacro, "macro", "security.allowMacro", "macro_disabled"},
	CapScreencast:       {CapScreencast, "screencast", "security.allowScreencast", "screencast_disabled"},
	CapDownload:         {CapDownload, "download", "security.allowDownload", "download_disabled"},
	CapCookies:          {CapCookies, "cookies", "security.allowCookies", "cookies_disabled"},
	CapUpload:           {CapUpload, "upload", "security.allowUpload", "upload_disabled"},
	CapStateExport:      {CapStateExport, "stateExport", "security.allowStateExport", "state_export_disabled"},
	CapNetworkIntercept: {CapNetworkIntercept, "networkIntercept", "security.allowNetworkIntercept", "network_intercept_disabled"},
}

// Meta returns the gate metadata for a capability. The second result is false
// for CapNone or any capability without registered metadata.
func Meta(cap Capability) (CapabilityMeta, bool) {
	m, ok := capabilityMeta[cap]
	return m, ok
}

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

var coreEndpoints = []Endpoint{
	{"POST", "/navigate", "Navigate URL or create tab", CapNone, true},
	{"POST", "/back", "Go back", CapNone, true},
	{"POST", "/forward", "Go forward", CapNone, true},
	{"POST", "/reload", "Reload page", CapNone, true},

	{"GET", "/snapshot", "Accessibility snapshot", CapNone, true},
	{"GET", "/frame", "Get current frame scope", CapNone, true},
	{"POST", "/frame", "Set current frame scope", CapNone, true},
	{"GET", "/screenshot", "Page screenshot", CapNone, true},
	{"GET", "/annotate", "Inject or clear the persistent clickable annotation overlay", CapNone, true},
	{"GET", "/capture", "Paired screenshot + accessibility snapshot from the same DOM epoch", CapNone, true},
	{"GET", "/text", "Extract page text", CapNone, true},
	{"GET", "/title", "Read page title", CapNone, true},
	{"GET", "/url", "Read page URL", CapNone, true},
	{"GET", "/html", "Read page HTML", CapNone, true},
	{"GET", "/styles", "Read computed styles", CapNone, true},
	{"GET", "/value", "Read form element value by ref", CapNone, true},
	{"GET", "/attr", "Read element HTML attribute by ref", CapNone, true},
	{"GET", "/count", "Count elements matching selector", CapNone, true},
	{"GET", "/box", "Get element bounding box by ref", CapNone, true},
	{"GET", "/visible", "Check if element is visible by ref", CapNone, true},
	{"GET", "/enabled", "Check if element is enabled by ref", CapNone, true},
	{"GET", "/checked", "Check if element is checked by ref", CapNone, true},
	{"GET", "/pdf", "Export as PDF (GET)", CapNone, true},
	{"POST", "/pdf", "Export as PDF (POST)", CapNone, true},

	{"POST", "/action", "Single action", CapNone, true},
	{"POST", "/actions", "Batch actions", CapNone, true},
	{"POST", "/dialog", "Handle dialog", CapNone, true},
	{"POST", "/wait", "Wait for condition", CapNone, true},
	{"POST", "/find", "Find elements", CapNone, true},

	{"POST", "/tab", "Create or focus tab", CapNone, false},
	{"POST", "/close", "Close tab", CapNone, true},
	{"POST", "/lock", "Lock tab", CapNone, true},
	{"POST", "/unlock", "Unlock tab", CapNone, true},

	{"POST", "/handoff", "Pause tab for human handoff", CapNone, true},
	{"POST", "/resume", "Resume paused tab", CapNone, true},
	{"GET", "/handoff", "Get handoff status", CapNone, true},

	{"GET", "/cookies", "Get cookies", CapCookies, true},
	{"POST", "/cookies", "Set cookies", CapCookies, true},
	{"DELETE", "/cookies", "Clear all cookies", CapCookies, true},

	{"GET", "/metrics", "Runtime metrics", CapNone, true},
	{"GET", "/timing", "Page timing and Core Web Vitals", CapNone, true},
	{"GET", "/a11y/audit", "Accessibility findings and score", CapNone, true},
	{"POST", "/audit/page", "Audit a single page with browser enrichment", CapNone, false},
	{"POST", "/audit", "Run a multi-page site audit", CapNone, false},

	{"GET", "/network", "Network log", CapNone, true},
	{"GET", "/network/stream", "Network SSE stream", CapNone, true},
	{"GET", "/network/export", "Export HAR", CapNone, true},
	{"GET", "/network/export/stream", "Export HAR stream", CapNone, true},
	{"GET", "/network/{requestId}", "Single network request", CapNone, true},
	{"POST", "/network/clear", "Clear network log", CapNone, false},
	{"GET", "/network/route", "List interception rules for a tab", CapNetworkIntercept, true},
	{"POST", "/network/route", "Install an interception rule", CapNetworkIntercept, true},
	{"DELETE", "/network/route", "Remove interception rule(s)", CapNetworkIntercept, true},

	{"GET", "/console", "Console logs", CapNone, false},
	{"POST", "/console/clear", "Clear console logs", CapNone, false},
	{"GET", "/errors", "Error logs", CapNone, false},
	{"POST", "/errors/clear", "Clear error logs", CapNone, false},

	{"GET", "/clipboard/read", "Read clipboard", CapNone, false},
	{"POST", "/clipboard/write", "Write clipboard", CapNone, false},
	{"POST", "/clipboard/copy", "Copy to clipboard", CapNone, false},
	{"GET", "/clipboard/paste", "Paste from clipboard", CapNone, false},

	{"GET", "/stealth/status", "Stealth configuration status", CapNone, false},
	{"POST", "/fingerprint/rotate", "Rotate browser fingerprint", CapNone, false},

	{"GET", "/solvers", "List available solvers", CapNone, false},
	{"GET", "/config/autosolver", "Get autosolver runtime config", CapNone, false},
	{"POST", "/solve", "Run default solver", CapNone, true},
	{"POST", "/solve/{name}", "Run named solver", CapNone, true},

	{"POST", "/emulation/viewport", "Set browser viewport dimensions", CapNone, true},
	{"POST", "/emulation/geolocation", "Set geolocation", CapNone, true},
	{"POST", "/emulation/offline", "Enable/disable offline mode", CapNone, true},
	{"POST", "/emulation/headers", "Set extra HTTP headers", CapNone, true},
	{"POST", "/emulation/credentials", "Set HTTP auth credentials", CapNone, true},
	{"POST", "/emulation/media", "Emulate CSS media features", CapNone, true},

	{"POST", "/cache/clear", "Clear browser cache", CapNone, false},
	{"GET", "/cache/status", "Cache status", CapNone, false},

	// Storage operations are gated under stateExport because they access/mutate sensitive client-side state.
	{"POST", "/storage", "Set storage item", CapStateExport, true},
	{"DELETE", "/storage", "Delete storage items", CapStateExport, true},

	{"GET", "/state", "Read current browser state", CapStateExport, false},
	{"GET", "/state/list", "List saved states", CapStateExport, false},

	{"POST", "/evaluate", "Run JavaScript in page", CapEvaluate, true},
	{"POST", "/macro", "Macro action pipeline", CapMacro, false},
	{"GET", "/download", "Download URL via browser session", CapDownload, true},
	{"POST", "/upload", "Upload file to file input", CapUpload, true},
	{"GET", "/screencast", "Live tab frame stream", CapScreencast, false},
	{"GET", "/screencast/tabs", "List tabs available for screencast", CapScreencast, false},
	{"POST", "/record/start", "Start recording browser activity to video", CapScreencast, false},
	{"POST", "/record/stop", "Stop recording and return encoded file", CapScreencast, false},
	{"GET", "/record/status", "Check recording status", CapScreencast, false},
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
	// /state is intentionally split: GET /state is capability-gated full browser
	// state, while GET /tabs/{id}/state is the lightweight ungated tab-runtime
	// readiness view.
	routes = append(routes, "GET /tabs/{id}/state")
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
