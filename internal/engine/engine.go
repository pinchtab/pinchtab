// Package engine provides a routing layer that decides whether to fulfil
// a request using the lightweight Gost-DOM engine ("lite") or Chrome (via CDP).
//
// The Router is the single entry point for handler code.  Route rules are
// pluggable: callers add / remove rules without touching handlers, bridge,
// or any other package.
package engine

import "context"

// Capability identifies an operation the engine may handle.
type Capability string

const (
	CapNavigate   Capability = "navigate"
	CapSnapshot   Capability = "snapshot"
	CapText       Capability = "text"
	CapClick      Capability = "click"
	CapType       Capability = "type"
	CapScreenshot Capability = "screenshot"
	CapPDF        Capability = "pdf"
	CapEvaluate   Capability = "evaluate"
	CapCookies    Capability = "cookies"
)

// Mode controls the engine selection strategy.
type Mode string

const (
	ModeChrome     Mode = "chrome"     // always Chrome (default)
	ModeLite       Mode = "lite"       // always lite (screenshot/pdf return 501)
	ModeLightpanda Mode = "lightpanda" // Lightpanda CDP engine
	ModeAuto       Mode = "auto"       // per-request routing via rules
)

// Decision is the routing verdict returned by a RouteRule.
type Decision int

const (
	Undecided     Decision = iota // rule has no opinion
	UseLite                       // route to Gost-DOM
	UseChrome                     // route to Chrome
	UseLightpanda                 // route to Lightpanda
)

// NavigateResult is the response from a navigation.
type NavigateResult struct {
	TabID string `json:"tabId"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

// SnapshotNode represents a single node in the accessibility-style snapshot.
type SnapshotNode struct {
	Ref         string `json:"ref"`
	Role        string `json:"role"`
	Name        string `json:"name"`
	Tag         string `json:"tag,omitempty"`
	Value       string `json:"value,omitempty"`
	Depth       int    `json:"depth"`
	Interactive bool   `json:"interactive,omitempty"`
}

// Engine is the minimal interface both lite and chrome wrappers implement.
type Engine interface {
	Name() string
	Navigate(ctx context.Context, url string) (*NavigateResult, error)
	Snapshot(ctx context.Context, filter string) ([]SnapshotNode, error)
	Text(ctx context.Context) (string, error)
	Click(ctx context.Context, ref string) error
	Type(ctx context.Context, ref, text string) error
	Capabilities() []Capability
	Close() error
}
