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
	"github.com/pinchtab/pinchtab/internal/runtimetypes"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

type TabTarget struct {
	TargetID string `json:"targetId"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Type     string `json:"type"`
}

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

	EnsureBrowser(cfg *config.RuntimeConfig) error
	RestartBrowser(cfg *config.RuntimeConfig) error
	// RunningBrowser reports the provider of the live browser process, or
	// ("", false) when nothing is running. EnsureBrowser ignores the requested
	// config once initialized, so handlers use this to reject explicit
	// browser params that the running process cannot honor.
	RunningBrowser() (string, bool)
	StealthStatus() *stealth.Status

	GetMemoryMetrics(tabID string) (*MemoryMetrics, error)
	GetBrowserMemoryMetrics() (*MemoryMetrics, error)
	GetAggregatedMemoryMetrics() (*MemoryMetrics, error)

	GetCrashLogs() []string

	NetworkMonitor() *NetworkMonitor

	AddRouteRule(tabID string, rule RouteRule) error
	RemoveRouteRule(tabID, pattern string) (int, error)
	ListRouteRules(tabID string) ([]RouteRule, error)

	GetDialogManager() *DialogManager

	GetConsoleLogs(tabID string, limit int) []LogEntry
	ClearConsoleLogs(tabID string)
	GetErrorLogs(tabID string, limit int) []ErrorEntry
	ClearErrorLogs(tabID string)

	Navigate(ctx context.Context, url string, params NavigateParams) (*NavigateResult, error)

	Snapshot(ctx context.Context, tabID string, filter string, params ContentParams) (*SnapshotResult, error)

	Text(ctx context.Context, tabID string, params ContentParams) (*TextResult, error)

	ClearCache(ctx context.Context) error
	CanClearCache(ctx context.Context) (bool, error)

	ClearCookies(ctx context.Context) error

	Evaluate(ctx context.Context, expression string, result any, opts EvalOpts) error

	// CallFunctionOnNode resolves a backend node ID to a Runtime object,
	// then calls the given JavaScript function on it. args may be nil.
	// The result is unmarshaled from the CDP returnByValue response.
	CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error

	// EvaluateInFrame evaluates a JavaScript expression in the given
	// frame's execution context. If frameID is empty, behaves like Evaluate.
	EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts EvalOpts) error

	DescribeNode(ctx context.Context, backendNodeID int64) (*NodeInfo, error)

	CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error)

	StartScreencast(ctx context.Context, opts ScreencastOpts) (*ScreencastStream, error)

	SetViewport(ctx context.Context, params ViewportParams) error
	SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error
	SetEmulatedMedia(ctx context.Context, feature, value string) error

	SetNetworkConditions(ctx context.Context, params NetworkConditions) error
	SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error
	GetCookies(ctx context.Context, urls []string) ([]CookieData, error)
	SetCookie(ctx context.Context, params SetCookieParams) error

	CurrentURL(ctx context.Context) (string, error)
	CurrentTitle(ctx context.Context) (string, error)

	PrintToPDF(ctx context.Context, params PDFParams) ([]byte, error)

	SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error
	ResolveSelectorToNodeID(ctx context.Context, selector string) (int64, error)

	DownloadURL(ctx context.Context, dlURL string, opts DownloadOpts) (*DownloadResult, error)

	EnableFetchWithAuth(ctx context.Context) error
	DisableFetch(ctx context.Context) error
	ListenAuthRequired(ctx context.Context, handler func(requestID string, isAuth bool))
	ContinueWithAuth(ctx context.Context, requestID, username, password string) error
	ContinueRequest(ctx context.Context, requestID string) error
	// SetFetchPauseSuppressed toggles per-tab suppression of the proxy-auth
	// listener's blanket ContinueRequest while the credentials handler owns
	// request-pause dispatch. Declared here (not probed) so decorated bridges
	// forward it instead of silently dropping the suppression contract.
	SetFetchPauseSuppressed(tabID string, v bool)

	GoBack(ctx context.Context) (didNavigate bool, err error)
	GoForward(ctx context.Context) (didNavigate bool, err error)
	Reload(ctx context.Context) error

	WaitVisible(ctx context.Context, selector string) error

	EnableNetwork(ctx context.Context) error
	ListenNetworkEvents(ctx context.Context, handler NetworkEventHandler)

	SetRawCookie(ctx context.Context, params RawSetCookieParams) error
	GetRawCookies(ctx context.Context) ([]RawCookie, error)

	SetUserAgentOverride(ctx context.Context, params UserAgentOverrideParams) error
	SetLocaleOverride(ctx context.Context, locale string) error
	SetTimezoneOverride(ctx context.Context, timezoneID string) error
	SetDeviceMetricsOverride(ctx context.Context, params DeviceMetricsOverrideParams) error
	AddScriptToEvaluateOnNewDocument(ctx context.Context, source string) (string, error)
}

// Browser-runtime DTOs are defined once in internal/runtimetypes and aliased
// here so bridge and the browsers RuntimeInstance speak one set of structs.
// Construct ScreencastStream via runtimetypes.NewScreencastStreamWithDone (the
// bridge screencast loop owns and closes the done channel itself).
type (
	EvalOpts          = runtimetypes.EvalOpts
	NodeInfo          = runtimetypes.NodeInfo
	ScreencastOpts    = runtimetypes.ScreencastOpts
	ScreencastStream  = runtimetypes.ScreencastStream
	CookieData        = runtimetypes.CookieData
	SetCookieParams   = runtimetypes.SetCookieParams
	ViewportParams    = runtimetypes.ViewportParams
	NetworkConditions = runtimetypes.NetworkConditions
	DownloadOpts      = runtimetypes.DownloadOpts
	DownloadResult    = runtimetypes.DownloadResult
	PDFParams         = runtimetypes.PDFParams
)

type LockInfo = bridgetabs.LockInfo

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

type UserAgentOverrideParams struct {
	UserAgent      string
	Platform       string
	AcceptLanguage string
}

type DeviceMetricsOverrideParams struct {
	Width             int64
	Height            int64
	DeviceScaleFactor float64
	Mobile            bool
	ScreenWidth       int64
	ScreenHeight      int64
}

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

type ProfileInfo struct {
	ID                string    `json:"id,omitempty"`
	Name              string    `json:"name"`
	Path              string    `json:"path,omitempty"`
	PathExists        bool      `json:"pathExists,omitempty"`
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
	ID             string          `json:"id"`            // Hash-based ID: inst_XXXXXXXX
	ProfileID      string          `json:"profileId"`     // Hash-based profile ID: prof_XXXXXXXX
	ProfileName    string          `json:"profileName"`   // Human-readable profile name (for display only)
	Port           string          `json:"port"`          // Internal: instance port
	URL            string          `json:"url,omitempty"` // Canonical base URL for bridge-backed instances
	Mode           string          `json:"mode"`          // API mode: "headless" or "headed"
	Headless       bool            `json:"headless"`      // Mode: headless vs headed
	Status         string          `json:"status"`        // Status: starting/running/stopping/stopped/error
	StartTime      time.Time       `json:"startTime"`
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
