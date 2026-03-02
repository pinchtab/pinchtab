// Package primitive defines the core interfaces for browser resource management.
// Strategies compose these primitives to implement different allocation patterns.
package primitive

import (
	"context"
	"time"
)

// Instance represents a running browser instance.
type Instance struct {
	ID        string    `json:"id"`
	Profile   string    `json:"profile"`
	Port      int       `json:"port"`
	Headless  bool      `json:"headless"`
	Status    string    `json:"status"` // starting, running, stopped
	StartedAt time.Time `json:"startedAt"`
	PID       int       `json:"pid,omitempty"`
}

// BaseURL returns the HTTP URL for this instance.
func (i *Instance) BaseURL() string {
	return "http://127.0.0.1:" + string(rune(i.Port))
}

// Tab represents an open browser tab.
type Tab struct {
	ID         string `json:"id"`         // tab_xxx hash format
	InstanceID string `json:"instanceId"` // inst_xxx
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// Profile represents a browser profile.
type Profile struct {
	ID        string            `json:"id"`   // prof_xxx
	Name      string            `json:"name"` // human-readable
	Path      string            `json:"path,omitempty"`
	Account   string            `json:"account,omitempty"`
	UseWhen   string            `json:"useWhen,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	CreatedAt time.Time         `json:"createdAt,omitempty"`
}

// InstanceManager controls browser instance lifecycle.
type InstanceManager interface {
	// Launch starts a new browser instance with the given profile.
	Launch(ctx context.Context, profile string, port int, headless bool) (*Instance, error)

	// Stop terminates a running instance.
	Stop(ctx context.Context, id string) error

	// List returns all instances (running and starting).
	List() []*Instance

	// Get returns a specific instance by ID.
	Get(id string) (*Instance, bool)

	// FirstRunning returns the first running instance, or nil.
	FirstRunning() *Instance

	// WaitReady blocks until instance is ready or context expires.
	WaitReady(ctx context.Context, id string) error
}

// NavigateOpts configures navigation behavior.
type NavigateOpts struct {
	Timeout     time.Duration `json:"timeout,omitempty"`
	WaitUntil   string        `json:"waitUntil,omitempty"` // load, domcontentloaded, networkidle
	BlockImages bool          `json:"blockImages,omitempty"`
	BlockMedia  bool          `json:"blockMedia,omitempty"`
	BlockAds    bool          `json:"blockAds,omitempty"`
}

// SnapshotOpts configures snapshot output.
type SnapshotOpts struct {
	Interactive bool   `json:"interactive,omitempty"` // only interactive elements
	Compact     bool   `json:"compact,omitempty"`     // compact format
	Format      string `json:"format,omitempty"`      // json, yaml, text
	Depth       int    `json:"depth,omitempty"`       // max tree depth
	MaxTokens   int    `json:"maxTokens,omitempty"`   // truncate output
	Selector    string `json:"selector,omitempty"`    // CSS selector scope
	Diff        bool   `json:"diff,omitempty"`        // only changes since last
}

// Snapshot represents the accessibility tree output.
type Snapshot struct {
	Nodes     []map[string]any `json:"nodes,omitempty"`
	Tree      map[string]any   `json:"tree,omitempty"`
	Text      string           `json:"text,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
}

// Action represents a browser action (click, type, etc).
type Action struct {
	Kind     string `json:"kind"`               // click, type, press, hover, scroll, fill, select, focus
	Ref      string `json:"ref,omitempty"`      // element ref from snapshot (e0, e1...)
	Selector string `json:"selector,omitempty"` // CSS selector alternative
	Text     string `json:"text,omitempty"`     // for type/fill
	Key      string `json:"key,omitempty"`      // for press
	Value    string `json:"value,omitempty"`    // for select
	X        int    `json:"x,omitempty"`        // for scroll
	Y        int    `json:"y,omitempty"`        // for scroll
	WaitNav  bool   `json:"waitNav,omitempty"`  // wait for navigation after action
}

// ActionResult contains action execution results.
type ActionResult struct {
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	Navigated bool   `json:"navigated,omitempty"`
}

// ScreenshotOpts configures screenshot capture.
type ScreenshotOpts struct {
	Format   string `json:"format,omitempty"`   // png, jpeg
	Quality  int    `json:"quality,omitempty"`  // jpeg quality 0-100
	FullPage bool   `json:"fullPage,omitempty"` // capture entire page
	Selector string `json:"selector,omitempty"` // element to capture
}

// PDFOpts configures PDF export.
type PDFOpts struct {
	Landscape           bool    `json:"landscape,omitempty"`
	PrintBackground     bool    `json:"printBackground,omitempty"`
	Scale               float64 `json:"scale,omitempty"`
	PaperWidth          float64 `json:"paperWidth,omitempty"`
	PaperHeight         float64 `json:"paperHeight,omitempty"`
	MarginTop           float64 `json:"marginTop,omitempty"`
	MarginBottom        float64 `json:"marginBottom,omitempty"`
	MarginLeft          float64 `json:"marginLeft,omitempty"`
	MarginRight         float64 `json:"marginRight,omitempty"`
	PageRanges          string  `json:"pageRanges,omitempty"`
	DisplayHeaderFooter bool    `json:"displayHeaderFooter,omitempty"`
	HeaderTemplate      string  `json:"headerTemplate,omitempty"`
	FooterTemplate      string  `json:"footerTemplate,omitempty"`
}

// TextOpts configures text extraction.
type TextOpts struct {
	Raw bool `json:"raw,omitempty"` // raw text vs readability-cleaned
}

// TextResult contains extracted text.
type TextResult struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Cookie represents an HTTP cookie.
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
}

// TabManager controls tabs within instances.
type TabManager interface {
	// Open creates a new tab in the specified instance.
	Open(ctx context.Context, instanceID string, url string) (tabID string, err error)

	// Close closes a tab.
	Close(ctx context.Context, tabID string) error

	// List returns all tabs for an instance.
	List(ctx context.Context, instanceID string) ([]*Tab, error)

	// ListAll returns all tabs across all instances.
	ListAll(ctx context.Context) ([]*Tab, error)

	// Get returns a specific tab.
	Get(tabID string) (*Tab, bool)

	// FindInstance returns the instance ID for a tab.
	FindInstance(tabID string) (instanceID string, ok bool)

	// Navigate loads a URL in a tab.
	Navigate(ctx context.Context, tabID string, url string, opts NavigateOpts) error

	// Snapshot returns the accessibility tree for a tab.
	Snapshot(ctx context.Context, tabID string, opts SnapshotOpts) (*Snapshot, error)

	// Action performs a browser action in a tab.
	Action(ctx context.Context, tabID string, action Action) (*ActionResult, error)

	// Actions performs multiple actions in sequence.
	Actions(ctx context.Context, tabID string, actions []Action) ([]*ActionResult, error)

	// Screenshot captures a tab screenshot.
	Screenshot(ctx context.Context, tabID string, opts ScreenshotOpts) ([]byte, error)

	// PDF exports a tab as PDF.
	PDF(ctx context.Context, tabID string, opts PDFOpts) ([]byte, error)

	// Text extracts readable text from a tab.
	Text(ctx context.Context, tabID string, opts TextOpts) (*TextResult, error)

	// Evaluate runs JavaScript in a tab.
	Evaluate(ctx context.Context, tabID string, expr string) (any, error)

	// Cookies returns cookies for a tab.
	Cookies(ctx context.Context, tabID string) ([]*Cookie, error)

	// SetCookies sets cookies for a tab.
	SetCookies(ctx context.Context, tabID string, url string, cookies []*Cookie) error

	// Lock acquires exclusive access to a tab.
	Lock(ctx context.Context, tabID string, owner string, ttl time.Duration) error

	// Unlock releases exclusive access to a tab.
	Unlock(ctx context.Context, tabID string, owner string) error
}

// ProfileManager controls browser profiles.
type ProfileManager interface {
	// List returns all profiles.
	List() ([]*Profile, error)

	// Get returns a specific profile.
	Get(name string) (*Profile, error)

	// Create creates a new profile.
	Create(name string) error

	// Delete removes a profile.
	Delete(name string) error

	// Exists checks if a profile exists.
	Exists(name string) bool

	// Reset clears profile data (cookies, cache, etc).
	Reset(name string) error

	// Import copies an existing Chrome profile.
	Import(name string, sourcePath string) error
}

// Primitives bundles all managers for dependency injection.
type Primitives struct {
	Instances InstanceManager
	Tabs      TabManager
	Profiles  ProfileManager
}
