// Package engine provides a routing layer that decides whether to fulfil
// a request using the lightweight Gost-DOM engine ("lite") or Chrome (via CDP).
//
// The Router is the single entry point for handler code.  Route rules are
// pluggable: callers add / remove rules without touching handlers, bridge,
// or any other package.
package engine

import (
	"context"
	"errors"
	"fmt"
)

// IDPIBlockedError is returned when IDPI security checks block a request.
type IDPIBlockedError struct {
	Reason string
}

func (e *IDPIBlockedError) Error() string {
	return fmt.Sprintf("blocked by IDPI: %s", e.Reason)
}

// IsIDPIBlocked reports whether err is an IDPI block.
func IsIDPIBlocked(err error) bool {
	var target *IDPIBlockedError
	return errors.As(err, &target)
}

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
	ModeChrome Mode = "chrome" // always Chrome (default)
	ModeLite   Mode = "lite"   // always lite (screenshot/pdf return 501)
	ModeAuto   Mode = "auto"   // per-request routing via rules
)

// Decision is the routing verdict returned by a RouteRule.
type Decision int

const (
	Undecided Decision = iota // rule has no opinion
	UseLite                   // route to Gost-DOM
	UseChrome                 // route to Chrome
)

// NavigateResult is the response from a navigation.
type NavigateResult struct {
	TabID  string `json:"tabId"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Engine string `json:"engine,omitempty"` // which engine fulfilled the request
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

// SnapshotResult is the response from a snapshot operation.
type SnapshotResult struct {
	Nodes       []SnapshotNode `json:"nodes"`
	URL         string         `json:"url,omitempty"`
	Title       string         `json:"title,omitempty"`
	Engine      string         `json:"engine,omitempty"`
	IDPIWarning string         `json:"idpiWarning,omitempty"`
}

// TextResult is the response from a text extraction operation.
type TextResult struct {
	Text      string `json:"text"`
	URL       string `json:"url,omitempty"`
	Title     string `json:"title,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Engine    string `json:"engine,omitempty"`
}

// ActionResult is the response from a click/type/other action.
type ActionResult struct {
	Data   map[string]any `json:"data,omitempty"`
	Engine string         `json:"engine,omitempty"`
}

// Engine is the minimal interface both lite and chrome wrappers implement.
type Engine interface {
	Name() string
	Navigate(ctx context.Context, url string) (*NavigateResult, error)
	Snapshot(ctx context.Context, tabID, filter string) (*SnapshotResult, error)
	Text(ctx context.Context, tabID string) (*TextResult, error)
	Click(ctx context.Context, tabID, ref string) error
	Type(ctx context.Context, tabID, ref, text string) error
	Capabilities() []Capability
	Close() error
}
