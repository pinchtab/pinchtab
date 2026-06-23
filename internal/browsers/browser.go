// Package browsers defines the Browser interface and a thread-safe global
// registry. Each browser implementation (chrome, cloak, …) registers itself
// via an init() function so callers never need to know the concrete types.
package browsers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type LaunchErrorKind int

const (
	LaunchErrorUnknown LaunchErrorKind = iota
	LaunchErrorSilentCDPDrop
)

type LaunchFailure struct {
	Err             error
	Elapsed         time.Duration
	ParentCanceled  bool
	BrowserCanceled bool
}

// Routing contract:
//
// DecisionHandle — provider accepts the request; proceed with execution.
// DecisionSkip   — provider cannot serve this shape/intent; caller may try
//                  another provider. Must NOT be used for security denials.
// DecisionFail   — fatal provider error; abort immediately.
//
// Security policy (domain blocks, IDPI, private IP) is enforced separately
// at the handler level and is never subject to fallback.

type Decision string

const (
	DecisionHandle Decision = "handle"
	DecisionSkip   Decision = "skip"
	DecisionFail   Decision = "fail"
)

type HandleDecision struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
}

type RequestIntent struct {
	Shape         string `json:"shape"`
	StateChanging bool   `json:"stateChanging,omitempty"`
}

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

type Browser interface {
	// ID returns a short stable identifier ("chrome", "cloak", …).
	ID() string
	DisplayName() string
	// Capabilities reports implementation-level features this browser
	// supports (e.g. CDP, headless, native stealth). Used for launch
	// configuration and diagnostics — not for routing decisions; use
	// CanHandle for that.
	Capabilities() CapabilitySet

	DiscoverBinary() BinaryDiscovery
	DoctorChecks(cfg TargetConfig) []DoctorCheck

	BuildLaunchArgs(cfg LaunchConfig) ([]string, []string, error)
	SupportsRemoteCDP() bool

	GeoAlignment(geo GeoConfig) GeoStrategy
	ValidateTarget(cfg TargetConfig) error

	// ClassifyLaunchError lets a provider identify known failure patterns
	// (e.g. silent CDP drops) so the runtime can apply targeted recovery.
	ClassifyLaunchError(f LaunchFailure) LaunchErrorKind

	// CanHandle reports whether this browser can serve the given request
	// intent. Providers return DecisionHandle, DecisionSkip, or
	// DecisionFail with an optional reason.
	CanHandle(intent RequestIntent) HandleDecision

	// NewRuntimeInstance creates a post-launch RuntimeInstance from an
	// already-initialized browser context. Each provider returns its own
	// Instance type (chrome, cloak, ghost-chrome).
	NewRuntimeInstance(browserCtx context.Context, headless bool) RuntimeInstance
}

var (
	mu       sync.Mutex
	registry = map[string]Browser{}
)

// Register panics if a browser with the same ID is already registered.
func Register(b Browser) {
	mu.Lock()
	defer mu.Unlock()

	id := b.ID()
	if _, exists := registry[id]; exists {
		panic(fmt.Sprintf("browsers: duplicate registration for %q", id))
	}
	registry[id] = b
}

func Get(id string) (Browser, bool) {
	mu.Lock()
	defer mu.Unlock()

	b, ok := registry[id]
	return b, ok
}

// MustGet panics with a message listing all known IDs if id is not found.
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

func IDs() []string {
	mu.Lock()
	defer mu.Unlock()

	return sortedKeysLocked()
}

// Caller must hold mu.
func sortedKeysLocked() []string {
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func resetForTesting() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Browser{}
}
