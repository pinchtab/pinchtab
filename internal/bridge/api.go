package bridge

import (
	"context"
	"net/http"
	"time"

	"github.com/chromedp/cdproto/target"
	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

// BridgeAPI abstracts browser tab operations for handler testing.
type BridgeAPI interface {
	BrowserContext() context.Context
	TabContext(tabID string) (ctx context.Context, resolvedID string, err error)
	ListTargets() ([]*target.Info, error)
	CreateTab(url string) (tabID string, ctx context.Context, cancel context.CancelFunc, err error)
	CloseTab(tabID string) error
	FocusTab(tabID string) error

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
	StealthStatus() *stealth.Status

	// Memory metrics
	GetMemoryMetrics(tabID string) (*MemoryMetrics, error)
	GetBrowserMemoryMetrics() (*MemoryMetrics, error)
	GetAggregatedMemoryMetrics() (*MemoryMetrics, error)

	// Crash monitoring
	GetCrashLogs() []string

	// Network monitoring
	NetworkMonitor() *NetworkMonitor

	// Dialog management
	GetDialogManager() *DialogManager

	// Console and error logs
	GetConsoleLogs(tabID string, limit int) []LogEntry
	ClearConsoleLogs(tabID string)
	GetErrorLogs(tabID string, limit int) []ErrorEntry
	ClearErrorLogs(tabID string)
}

type LockInfo = bridgetabs.LockInfo

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

type Instance struct {
	ID          string    `json:"id"`                   // Hash-based ID: inst_XXXXXXXX
	ProfileID   string    `json:"profileId"`            // Hash-based profile ID: prof_XXXXXXXX
	ProfileName string    `json:"profileName"`          // Human-readable profile name (for display only)
	Port        string    `json:"port"`                 // Internal: instance port
	URL         string    `json:"url,omitempty"`        // Canonical base URL for bridge-backed instances
	Headless    bool      `json:"headless"`             // Mode: headless vs headed
	Status      string    `json:"status"`               // Status: starting/running/stopping/stopped/error
	StartTime   time.Time `json:"startTime"`            // When instance was created
	Error       string    `json:"error,omitempty"`      // Error message if status=error
	Attached    bool      `json:"attached"`             // True if attached rather than locally launched
	AttachType  string    `json:"attachType,omitempty"` // "cdp" or "bridge" for attached instances
	CdpURL      string    `json:"cdpUrl,omitempty"`     // CDP WebSocket URL (for CDP-attached instances)
}

type InstanceTab struct {
	ID         string `json:"id"`         // Runtime tab ID (raw CDP target ID on this branch)
	InstanceID string `json:"instanceId"` // Hash-based instance ID: inst_XXXXXXXX
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// test
