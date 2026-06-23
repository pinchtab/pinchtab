// Package browserops defines browser operation contracts and response types
// shared by handlers, routing, and provider runtimes.
package browserops

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

func IsIDPIBlocked(err error) bool {
	var target *IDPIBlockedError
	return errors.As(err, &target)
}

// StaticActionRequest is the cycle-safe subset of a full browser action request
// that static-routing layers (ghost-chrome's BridgeProxy) need to decide
// whether the lightweight static browser can serve an action by ref. It lives
// here, in a leaf package both bridge and the ghost-chrome packages import, so
// the subset is defined once instead of being hand-copied to dodge the
// bridge → config → browsers/all → ghostchrome import cycle. The richer
// bridge.ActionRequest is mapped down to this at the adapter boundary.
type StaticActionRequest struct {
	TabID string
	Kind  string
	Ref   string
	Text  string
	Value string
}

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
	CapCapture    Capability = "capture"
)

type NavigateResult struct {
	TabID string         `json:"tabId"`
	URL   string         `json:"url"`
	Title string         `json:"title"`
	Route *RouteMetadata `json:"route,omitempty"`
}

type SnapshotNode struct {
	Ref         string `json:"ref"`
	Role        string `json:"role"`
	Name        string `json:"name"`
	Tag         string `json:"tag,omitempty"`
	Value       string `json:"value,omitempty"`
	Depth       int    `json:"depth"`
	Interactive bool   `json:"interactive,omitempty"`
}

type SnapshotResult struct {
	Nodes       []SnapshotNode `json:"nodes"`
	URL         string         `json:"url,omitempty"`
	Title       string         `json:"title,omitempty"`
	Route       *RouteMetadata `json:"route,omitempty"`
	IDPIWarning string         `json:"idpiWarning,omitempty"`
}

type TextResult struct {
	Text      string         `json:"text"`
	URL       string         `json:"url,omitempty"`
	Title     string         `json:"title,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
	Route     *RouteMetadata `json:"route,omitempty"`
}

type ActionResult struct {
	Data  map[string]any `json:"data,omitempty"`
	Route *RouteMetadata `json:"route,omitempty"`
}

type RouteMetadata struct {
	RequestedBrowser string         `json:"requestedProvider"`
	UsedBrowser      string         `json:"usedProvider"`
	Escalated        bool           `json:"escalated"`
	Reason           string         `json:"reason,omitempty"`
	Quality          int            `json:"quality,omitempty"`
	FallbackAttempts int            `json:"fallbackAttempts,omitempty"`
	Attempts         []RouteAttempt `json:"attempts,omitempty"`
}

type RouteAttempt struct {
	Browser  string `json:"provider"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

func SingleBrowserRoute(browser string) *RouteMetadata {
	return &RouteMetadata{
		RequestedBrowser: browser,
		UsedBrowser:      browser,
		Attempts: []RouteAttempt{
			{Browser: browser, Accepted: true},
		},
	}
}

// BrowserRuntime is the minimal interface implemented by browser operation
// runtimes such as static fetch or browser-backed CDP adapters.
type BrowserRuntime interface {
	Name() string
	Navigate(ctx context.Context, url string) (*NavigateResult, error)
	Snapshot(ctx context.Context, tabID, filter string) (*SnapshotResult, error)
	Text(ctx context.Context, tabID string) (*TextResult, error)
	Click(ctx context.Context, tabID, ref string) error
	Type(ctx context.Context, tabID, ref, text string) error
	Capabilities() []Capability
	Close() error
}
