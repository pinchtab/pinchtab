// Package browsers defines the Browser interface and a thread-safe global
// registry. Each browser implementation (chrome, cloak, …) registers itself
// via an init() function so callers never need to know the concrete types.
package browsers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// LaunchErrorKind classifies a browser launch failure.
type LaunchErrorKind int

const (
	LaunchErrorUnknown LaunchErrorKind = iota
	LaunchErrorSilentCDPDrop
)

// LaunchFailure carries context about a browser launch error for provider
// classification.
type LaunchFailure struct {
	Err             error
	Elapsed         time.Duration
	ParentCanceled  bool
	BrowserCanceled bool
}

// ---------------------------------------------------------------------------
// Handle-decision types
// ---------------------------------------------------------------------------

// Routing contract:
//
// DecisionHandle — provider accepts the request; proceed with execution.
// DecisionSkip   — provider cannot serve this shape/intent; caller may try
//                  another provider. Must NOT be used for security denials.
// DecisionFail   — fatal provider error; abort immediately.
//
// Security policy (domain blocks, IDPI, private IP) is enforced separately
// at the handler level and is never subject to fallback.

// Decision classifies whether a browser can handle a given request intent.
type Decision string

const (
	DecisionHandle Decision = "handle"
	DecisionSkip   Decision = "skip"
	DecisionFail   Decision = "fail"
)

// HandleDecision pairs a Decision with an optional human-readable reason.
type HandleDecision struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
}

// RequestIntent describes the shape of an incoming request so a browser can
// decide whether it is able to serve it.
type RequestIntent struct {
	Shape         string `json:"shape"`
	StateChanging bool   `json:"stateChanging,omitempty"`
}

// Shape constants classify the kind of work a request represents.
const (
	ShapeStaticRead     = "static-read"
	ShapeStaticSnapshot = "static-snapshot"
	ShapeRenderedRead   = "rendered-read"
	ShapeVisual         = "visual"
	ShapeInteraction    = "interaction"
	ShapeSessionState   = "session-state"
	ShapeNetworkControl = "network-control"
	ShapeDownloadUpload = "download-upload"
)

// ---------------------------------------------------------------------------
// Browser interface
// ---------------------------------------------------------------------------

// Browser is the extension point for adding new browser providers.
type Browser interface {
	// ID returns a short stable identifier ("chrome", "cloak", …).
	ID() string
	// DisplayName returns a human-readable label.
	DisplayName() string
	// Capabilities reports implementation-level features this browser
	// supports (e.g. CDP, headless, native stealth). Used for launch
	// configuration and diagnostics — not for routing decisions; use
	// CanHandle for that.
	Capabilities() CapabilitySet

	// DiscoverBinary locates the browser binary on the current system.
	DiscoverBinary() BinaryDiscovery
	// DoctorChecks returns provider-specific health checks.
	DoctorChecks(cfg TargetConfig) []DoctorCheck

	// BuildLaunchArgs produces the CLI args and env vars needed to start
	// the browser. Returns (args, env, err).
	BuildLaunchArgs(cfg LaunchConfig) ([]string, []string, error)
	// SupportsRemoteCDP reports whether this browser can be attached to
	// via a remote CDP endpoint.
	SupportsRemoteCDP() bool

	// GeoAlignment returns launch-time flags/env derived from geo config.
	GeoAlignment(geo GeoConfig) GeoStrategy
	// ValidateTarget checks that a target configuration is valid for this
	// browser, returning a descriptive error if not.
	ValidateTarget(cfg TargetConfig) error

	// ClassifyLaunchError lets a provider identify known failure patterns
	// (e.g. silent CDP drops) so the runtime can apply targeted recovery.
	ClassifyLaunchError(f LaunchFailure) LaunchErrorKind

	// CanHandle reports whether this browser can serve the given request
	// intent. Providers return DecisionHandle, DecisionSkip, or
	// DecisionFail with an optional reason.
	CanHandle(intent RequestIntent) HandleDecision
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

var (
	mu       sync.Mutex
	registry = map[string]Browser{}
)

// Register adds a Browser to the global registry.
// It panics if a browser with the same ID is already registered.
func Register(b Browser) {
	mu.Lock()
	defer mu.Unlock()

	id := b.ID()
	if _, exists := registry[id]; exists {
		panic(fmt.Sprintf("browsers: duplicate registration for %q", id))
	}
	registry[id] = b
}

// Get returns the Browser registered under id, or (nil, false) if unknown.
func Get(id string) (Browser, bool) {
	mu.Lock()
	defer mu.Unlock()

	b, ok := registry[id]
	return b, ok
}

// MustGet returns the Browser registered under id.
// It panics with a message listing all known IDs if id is not found.
func MustGet(id string) Browser {
	mu.Lock()
	defer mu.Unlock()

	if b, ok := registry[id]; ok {
		return b
	}

	known := sortedKeysLocked()
	panic(fmt.Sprintf(
		"browsers: unknown browser %q (known: [%s])",
		id, strings.Join(known, ", "),
	))
}

// IDs returns a sorted list of all registered browser IDs.
// It returns an empty (non-nil) slice when the registry is empty.
func IDs() []string {
	mu.Lock()
	defer mu.Unlock()

	return sortedKeysLocked()
}

// sortedKeysLocked returns sorted registry keys. Caller must hold mu.
func sortedKeysLocked() []string {
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// resetForTesting clears the global registry. Exported only for tests.
func resetForTesting() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Browser{}
}
