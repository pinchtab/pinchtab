package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

// TabTarget is a bridge-level representation of a browser tab/target,
// decoupling handlers from the cdproto/target package.
type TabTarget struct {
	TargetID string `json:"targetId"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Type     string `json:"type"`
}

// BridgeAPI abstracts browser tab operations for handler testing.
var ErrBrowserDraining = errors.New("browser restart in progress; retry shortly")

type BridgeAPI interface {
	BrowserContext() context.Context
	TabContext(tabID string) (ctx *TabHandle, resolvedID string, err error)
	ListTargets() ([]TabTarget, error)
	CreateTab(url string) (tabID string, ctx context.Context, cancel context.CancelFunc, err error)
	CloseTab(tabID string) error
	FocusTab(tabID string) error

	// ScheduleAutoClose (re)arms the per-tab idle close timer when the
	// lifecycle policy is "close_idle". No-op otherwise.
	ScheduleAutoClose(tabID string)
	// CancelAutoClose stops the per-tab idle close timer if any.
	CancelAutoClose(tabID string)

	GetRefCache(tabID string) *RefCache
	SetRefCache(tabID string, cache *RefCache)
	DeleteRefCache(tabID string)

	ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error)
	AvailableActions() []string

	// Execute runs a task for a tab with per-tab sequential execution
	// and cross-tab bounded parallelism. If not supported, runs directly.
	Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error

	TabLockInfo(tabID string) *LockInfo
	Lock(tabID, owner string, ttl time.Duration) error
	Unlock(tabID, owner string) error

	EnsureChrome(cfg *config.RuntimeConfig) error
	RestartBrowser(cfg *config.RuntimeConfig) error
	StealthStatus() *stealth.Status

	// Memory metrics
	GetMemoryMetrics(tabID string) (*MemoryMetrics, error)
	GetBrowserMemoryMetrics() (*MemoryMetrics, error)
	GetAggregatedMemoryMetrics() (*MemoryMetrics, error)

	// Crash monitoring
	GetCrashLogs() []string

	// Network monitoring
	NetworkMonitor() *NetworkMonitor

	// Network request interception (Fetch domain).
	AddRouteRule(tabID string, rule RouteRule) error
	RemoveRouteRule(tabID, pattern string) (int, error)
	ListRouteRules(tabID string) ([]RouteRule, error)

	// Dialog management
	GetDialogManager() *DialogManager

	// Console and error logs
	GetConsoleLogs(tabID string, limit int) []LogEntry
	ClearConsoleLogs(tabID string)
	GetErrorLogs(tabID string, limit int) []ErrorEntry
	ClearErrorLogs(tabID string)

	// Navigation
	Navigate(ctx context.Context, url string, params NavigateParams) (*NavigateResult, error)

	// Snapshot
	Snapshot(ctx context.Context, tabID string, filter string, params ContentParams) (*SnapshotResult, error)

	// Text
	Text(ctx context.Context, tabID string, params ContentParams) (*TextResult, error)

	// Cache management
	ClearCache(ctx context.Context) error
	CanClearCache(ctx context.Context) (bool, error)

	// Cookie management
	ClearCookies(ctx context.Context) error

	// JavaScript evaluation
	Evaluate(ctx context.Context, expression string, result any, opts EvalOpts) error

	// CallFunctionOnNode resolves a backend node ID to a Runtime object,
	// then calls the given JavaScript function on it. args may be nil.
	// The result is unmarshaled from the CDP returnByValue response.
	CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error

	// EvaluateInFrame evaluates a JavaScript expression in the given
	// frame's execution context. If frameID is empty, behaves like Evaluate.
	EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts EvalOpts) error

	// DescribeNode returns DOM structural info for a backend node ID.
	DescribeNode(ctx context.Context, backendNodeID int64) (*NodeInfo, error)

	// Screenshot capture
	CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error)

	// Screencast streaming
	StartScreencast(ctx context.Context, opts ScreencastOpts) (*ScreencastStream, error)

	// Emulation
	SetViewport(ctx context.Context, params ViewportParams) error
	SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error
	SetEmulatedMedia(ctx context.Context, feature, value string) error

	// Network state
	SetNetworkConditions(ctx context.Context, params NetworkConditions) error
	SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error
	GetCookies(ctx context.Context, urls []string) ([]CookieData, error)
	SetCookie(ctx context.Context, params SetCookieParams) error

	// Navigation info
	CurrentURL(ctx context.Context) (string, error)
	CurrentTitle(ctx context.Context) (string, error)

	// PDF generation
	PrintToPDF(ctx context.Context, params PDFParams) ([]byte, error)

	// DOM file input
	SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error
	ResolveSelectorToNodeID(ctx context.Context, selector string) (int64, error)

	// Download
	DownloadURL(ctx context.Context, dlURL string, opts DownloadOpts) (*DownloadResult, error)

	// HTTP auth credentials (Fetch domain)
	EnableFetchWithAuth(ctx context.Context) error
	DisableFetch(ctx context.Context) error
	ListenAuthRequired(ctx context.Context, handler func(requestID string, isAuth bool))
	ContinueWithAuth(ctx context.Context, requestID, username, password string) error
	ContinueRequest(ctx context.Context, requestID string) error

	// Navigation history
	GoBack(ctx context.Context) (didNavigate bool, err error)
	GoForward(ctx context.Context) (didNavigate bool, err error)
	Reload(ctx context.Context) error

	// Navigation wait helpers
	WaitVisible(ctx context.Context, selector string) error

	// Navigation policy (network guard)
	EnableNetwork(ctx context.Context) error
	ListenNetworkEvents(ctx context.Context, handler NetworkEventHandler)

	// State: cookie restore
	SetRawCookie(ctx context.Context, params RawSetCookieParams) error
	GetRawCookies(ctx context.Context) ([]RawCookie, error)

	// Stealth: fingerprint rotation
	SetUserAgentOverride(ctx context.Context, params UserAgentOverrideParams) error
	SetLocaleOverride(ctx context.Context, locale string) error
	SetTimezoneOverride(ctx context.Context, timezoneID string) error
	SetDeviceMetricsOverride(ctx context.Context, params DeviceMetricsOverrideParams) error
	AddScriptToEvaluateOnNewDocument(ctx context.Context, source string) (string, error)
}

// EvalOpts configures JavaScript evaluation behavior.
type EvalOpts struct {
	AwaitPromise bool
}

// NodeInfo holds DOM structural info returned by DescribeNode.
type NodeInfo struct {
	LocalName      string
	Attributes     []string
	ChildNodeCount int
}

// ScreencastOpts configures a screencast stream.
type ScreencastOpts struct {
	Quality       int // 1-100, default 30
	MaxWidth      int // pixels, default 800
	MaxHeight     int // pixels, default 600
	EveryNthFrame int // frame skipping for event mode, default 4
	FPS           int // frames per second (caps at 30), default 1
}

// ScreencastStream delivers decoded binary JPEG frames over a channel.
type ScreencastStream struct {
	Frames <-chan []byte
	done   chan struct{}
	closer func()
}

// Close stops the screencast and releases resources.
func (s *ScreencastStream) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	if s.closer != nil {
		s.closer()
	}
}

type LockInfo = bridgetabs.LockInfo

// NetworkEventHandler receives network events for navigation guards.
type NetworkEventHandler struct {
	OnRequestWillBeSent func(frameID, requestID string, resourceType string)
	OnResponseReceived  func(requestID string, remoteIPAddress string)
}

// RawSetCookieParams holds parameters for setting a cookie via CDP network domain,
// used by state.go to restore cookies from state files without importing cdproto.
type RawSetCookieParams struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Secure   bool
	HTTPOnly bool
	SameSite string // "Strict", "Lax", "None", or ""
}

// RawCookie represents a raw cookie from the CDP network domain.
type RawCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
}

// UserAgentOverrideParams holds parameters for user agent override.
type UserAgentOverrideParams struct {
	UserAgent      string
	Platform       string
	AcceptLanguage string
}

// DeviceMetricsOverrideParams holds parameters for device metrics override.
type DeviceMetricsOverrideParams struct {
	Width             int64
	Height            int64
	DeviceScaleFactor float64
	Mobile            bool
	ScreenWidth       int64
	ScreenHeight      int64
}

// ProfileService abstracts profile management operations.
type ProfileService interface {
	RegisterHandlers(mux *http.ServeMux)
	List() ([]ProfileInfo, error)
	Create(name string) error
	Import(name, sourcePath string) error
	Reset(name string) error
	Delete(name string) error
	Logs(name string, limit int) []ActionRecord
	Analytics(name string) AnalyticsReport
	RecordAction(profile string, record ActionRecord)
}

// OrchestratorService abstracts instance orchestration operations.
type OrchestratorService interface {
	RegisterHandlers(mux *http.ServeMux)
	Launch(name, port string, headless bool, extensionPaths []string) (*Instance, error)
	Stop(id string) error
	StopProfile(name string) error
	List() []Instance
	Logs(id string) (string, error)
	FirstRunningURL() string
	AllTabs() []InstanceTab
	FindInstanceByTab(tabID string) (*Instance, bool)
	ScreencastURL(instanceID, tabID string) string
	Shutdown()
	ForceShutdown()
}

// Common types used across packages (migrated from main)

type ProfileInfo struct {
	ID                string    `json:"id,omitempty"`
	Name              string    `json:"name"`
	Path              string    `json:"path,omitempty"`       // File system path to profile directory
	PathExists        bool      `json:"pathExists,omitempty"` // Whether the path exists on disk
	Created           time.Time `json:"created"`
	LastUsed          time.Time `json:"lastUsed"`
	DiskUsage         int64     `json:"diskUsage"`
	Running           bool      `json:"running"`
	Temporary         bool      `json:"temporary,omitempty"` // ephemeral instance profiles (auto-generated)
	Source            string    `json:"source,omitempty"`
	ChromeProfileName string    `json:"chromeProfileName,omitempty"`
	AccountEmail      string    `json:"accountEmail,omitempty"`
	AccountName       string    `json:"accountName,omitempty"`
	HasAccount        bool      `json:"hasAccount,omitempty"`
	UseWhen           string    `json:"useWhen,omitempty"`
	Description       string    `json:"description,omitempty"`
}

type ActionRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Endpoint   string    `json:"endpoint"`
	URL        string    `json:"url"`
	TabID      string    `json:"tabId"`
	DurationMs int64     `json:"durationMs"`
	Status     int       `json:"status"`
}

type AnalyticsReport struct {
	TotalActions   int            `json:"totalActions"`
	Last24h        int            `json:"last24h"`
	CommonHosts    map[string]int `json:"commonHosts"`
	TopEndpoints   map[string]int `json:"topEndpoints,omitempty"`
	RepeatPatterns []string       `json:"repeatPatterns,omitempty"`
	Suggestions    []string       `json:"suggestions,omitempty"`
}

type SecurityPolicy struct {
	AllowedDomains []string `json:"allowedDomains,omitempty"`
}

func ModeFromHeadless(headless bool) string {
	if headless {
		return "headless"
	}
	return "headed"
}

func normalizeInstanceMode(mode string, headless bool) string {
	switch mode {
	case "headless", "headed":
		return mode
	default:
		return ModeFromHeadless(headless)
	}
}

type Instance struct {
	ID             string          `json:"id"`                   // Hash-based ID: inst_XXXXXXXX
	ProfileID      string          `json:"profileId"`            // Hash-based profile ID: prof_XXXXXXXX
	ProfileName    string          `json:"profileName"`          // Human-readable profile name (for display only)
	Port           string          `json:"port"`                 // Internal: instance port
	URL            string          `json:"url,omitempty"`        // Canonical base URL for bridge-backed instances
	Mode           string          `json:"mode"`                 // API mode: "headless" or "headed"
	Headless       bool            `json:"headless"`             // Mode: headless vs headed
	Status         string          `json:"status"`               // Status: starting/running/stopping/stopped/error
	StartTime      time.Time       `json:"startTime"`            // When instance was created
	Error          string          `json:"error,omitempty"`      // Error message if status=error
	Attached       bool            `json:"attached"`             // True if attached rather than locally launched
	AttachType     string          `json:"attachType,omitempty"` // "cdp" or "bridge" for attached instances
	CdpURL         string          `json:"cdpUrl,omitempty"`     // CDP WebSocket URL (for CDP-attached instances)
	SecurityPolicy *SecurityPolicy `json:"securityPolicy,omitempty"`

	Browser string `json:"browser,omitempty"`

	// FallbackFrom/FallbackReason: omitempty keeps successful-launch
	// Instance JSON byte-identical to pre-P2.4a output.
	FallbackFrom   string `json:"fallbackFrom,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
}

func (i Instance) MarshalJSON() ([]byte, error) {
	type alias Instance
	copy := alias(i)
	copy.Mode = normalizeInstanceMode(copy.Mode, copy.Headless)
	return json.Marshal(copy)
}

type InstanceTab struct {
	ID         string `json:"id"`         // Runtime tab ID (raw CDP target ID on this branch)
	InstanceID string `json:"instanceId"` // Hash-based instance ID: inst_XXXXXXXX
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// test
