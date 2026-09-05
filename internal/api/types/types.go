// Package types contains the module's shared API wire types — read by the
// dashboard, the CLI and the MCP server. They are exported to TypeScript via
// tygo, so every exported declaration here must appear in the generated file.
package types

import (
	"time"
)

// Matches internal/bridge/api.go ProfileInfo
type Profile struct {
	ID                string    `json:"id,omitempty"`
	Name              string    `json:"name"`
	Path              string    `json:"path,omitempty"`
	PathExists        bool      `json:"pathExists,omitempty"`
	Created           time.Time `json:"created"`
	LastUsed          time.Time `json:"lastUsed"`
	DiskUsage         int64     `json:"diskUsage"`
	SizeMB            float64   `json:"sizeMB,omitempty"`
	Running           bool      `json:"running"`
	Temporary         bool      `json:"temporary,omitempty"`
	Quarantined       bool      `json:"quarantined,omitempty"`
	Source            string    `json:"source,omitempty"`
	ChromeProfileName string    `json:"chromeProfileName,omitempty"`
	AccountEmail      string    `json:"accountEmail,omitempty"`
	AccountName       string    `json:"accountName,omitempty"`
	HasAccount        bool      `json:"hasAccount,omitempty"`
	UseWhen           string    `json:"useWhen,omitempty"`
	Description       string    `json:"description,omitempty"`
}

type SecurityPolicy struct {
	AllowedDomains []string `json:"allowedDomains,omitempty"`
}

// Matches internal/bridge/api.go Instance
type Instance struct {
	ID             string          `json:"id"`
	ProfileID      string          `json:"profileId"`
	ProfileName    string          `json:"profileName"`
	Port           string          `json:"port"` // Note: string not int
	URL            string          `json:"url,omitempty"`
	Mode           string          `json:"mode"`
	Headless       bool            `json:"headless"`
	Status         string          `json:"status"` // starting/running/stopping/stopped/error
	StartTime      time.Time       `json:"startTime"`
	Error          string          `json:"error,omitempty"`
	Attached       bool            `json:"attached"` // True if attached rather than locally launched
	AttachType     string          `json:"attachType,omitempty"`
	CdpURL         string          `json:"cdpUrl,omitempty"` // CDP WebSocket URL (for CDP-attached instances)
	SecurityPolicy *SecurityPolicy `json:"securityPolicy,omitempty"`

	Browser string `json:"browser,omitempty"`

	// FallbackFrom/FallbackReason are reserved for a future phase (always
	// empty in P2.4a).
	FallbackFrom   string `json:"fallbackFrom,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`

	Crashes *CrashSummary `json:"crashes,omitempty"`
}

type CrashEvent struct {
	Time       time.Time `json:"time"`
	TargetID   string    `json:"targetId,omitempty"`
	TabID      string    `json:"tabId,omitempty"`
	URL        string    `json:"url,omitempty"`
	Reason     string    `json:"reason"`
	LastError  string    `json:"lastError,omitempty"`
	InstanceID string    `json:"instanceId,omitempty"`
}

type CrashSummary struct {
	Total  uint64       `json:"total"`
	Recent []CrashEvent `json:"recent"`
}

type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	ConnectedAt  time.Time `json:"connectedAt"`
	LastActivity time.Time `json:"lastActivity,omitempty"`
	RequestCount int       `json:"requestCount"`
}

type AgentDetail struct {
	Agent  Agent           `json:"agent"`
	Events []ActivityEvent `json:"events"`
}

type ActivityEvent struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agentId"`
	Channel   string                 `json:"channel"` // "tool_call" or "progress"
	Type      string                 `json:"type"`    // navigate/snapshot/action/screenshot/other
	Method    string                 `json:"method"`
	Path      string                 `json:"path"`
	Message   string                 `json:"message,omitempty"`  // human-readable (progress channel)
	Progress  *int                   `json:"progress,omitempty"` // 0-100 numeric progress
	Total     *int                   `json:"total,omitempty"`    // total steps
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type RouteAttempt struct {
	Browser  string `json:"provider"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
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

type ActivityLogEvent struct {
	Timestamp   time.Time      `json:"timestamp"`
	Source      string         `json:"source"`
	RequestID   string         `json:"requestId,omitempty"`
	SessionID   string         `json:"sessionId,omitempty"`
	AgentID     string         `json:"agentId,omitempty"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	Status      int            `json:"status"`
	DurationMs  int64          `json:"durationMs"`
	RemoteAddr  string         `json:"remoteAddr,omitempty"`
	InstanceID  string         `json:"instanceId,omitempty"`
	ProfileID   string         `json:"profileId,omitempty"`
	ProfileName string         `json:"profileName,omitempty"`
	TabID       string         `json:"tabId,omitempty"`
	URL         string         `json:"url,omitempty"`
	Action      string         `json:"action,omitempty"`
	Route       *RouteMetadata `json:"route,omitempty"`
	Ref         string         `json:"ref,omitempty"`
	Code        string         `json:"code,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type ActivityLogResponse struct {
	Events []ActivityLogEvent `json:"events"`
	Count  int                `json:"count"`
}

type ScreencastSettings struct {
	FPS      int `json:"fps"`
	Quality  int `json:"quality"`
	MaxWidth int `json:"maxWidth"`
}

type BrowserSettings struct {
	BlockImages  bool `json:"blockImages"`
	BlockMedia   bool `json:"blockMedia"`
	NoAnimations bool `json:"noAnimations"`
}

type Settings struct {
	Screencast ScreencastSettings `json:"screencast"`
	Stealth    string             `json:"stealth"` // light/medium/full
	Browser    BrowserSettings    `json:"browser"`
	Monitoring MonitoringSettings `json:"monitoring"`
	Agents     AgentSettings      `json:"agents"`
}

type AgentSettings struct {
	ReasoningMode string `json:"reasoningMode"` // "tool_calls" (default), "progress", "both"
}

type MonitoringSettings struct {
	MemoryMetrics bool `json:"memoryMetrics"` // Enable per-tab memory aggregation (can be heavy)
	PollInterval  int  `json:"pollInterval"`  // Poll interval in seconds (default 30)
}

type ServerInfo struct {
	Version   string `json:"version"`
	Uptime    int64  `json:"uptime"`
	Profiles  int    `json:"profiles"`
	Instances int    `json:"instances"`
	Agents    int    `json:"agents"`
}

type CreateProfileRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	UseWhen     string `json:"useWhen,omitempty"`
}

type CreateProfileResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

type InstanceTab struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

type InstanceMetrics struct {
	InstanceID  string  `json:"instanceId"`
	ProfileName string  `json:"profileName"`
	MemoryMB    float64 `json:"memoryMB"`
	Renderers   int     `json:"renderers"`

	Page              *PageMetrics `json:"page,omitempty"`
	UnreadableTargets int          `json:"unreadableTargets"`
}

type PageMetrics struct {
	Targets          int     `json:"targets"`
	JSHeapUsedMB     float64 `json:"jsHeapUsedMB"`
	JSHeapTotalMB    float64 `json:"jsHeapTotalMB"`
	Documents        int     `json:"documents"`
	Frames           int     `json:"frames"`
	Nodes            int     `json:"nodes"`
	JSEventListeners int     `json:"jsEventListeners"`
}

type LaunchInstanceRequest struct {
	ProfileID string `json:"profileId,omitempty"` // profile ID (prof_XXXXXXXX) or existing profile name
	Mode      string `json:"mode,omitempty"`      // "headed" or empty for headless
	Port      string `json:"port,omitempty"`      // port number as string
	Browser   string `json:"browser,omitempty"`   // browser override (chrome, cloak, ghost-chrome)
}

const ProfileStatusMissing = "missing"

type ProfileInstanceStatus struct {
	Name    string `json:"name"`
	Exists  bool   `json:"exists"`
	Running bool   `json:"running"`
	Status  string `json:"status"`
	Port    string `json:"port"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
}

// CaptureEnvelope is the /capture JSON envelope. The producer
// (internal/handlers.HandleCapture) still builds it as a map, so the parity test
// beside this type is what keeps the two descriptions equal; the CLI and the MCP
// server decode into this rather than each keeping a private mirror.
type CaptureEnvelope struct {
	Status           string          `json:"status"`
	TabID            string          `json:"tabId"`
	URL              string          `json:"url"`
	Title            string          `json:"title"`
	CapturedAt       string          `json:"capturedAt"`
	Epoch            CaptureEpoch    `json:"epoch"`
	Pairing          CapturePairing  `json:"pairing"`
	Image            CaptureImage    `json:"image"`
	Snapshot         CaptureSnapshot `json:"snapshot"`
	Frame            *CaptureFrame   `json:"frame,omitempty"`
	IDPIWarning      string          `json:"idpiWarning,omitempty"`
	UntrustedContent bool            `json:"untrustedContent,omitempty"`
	IDPINotice       string          `json:"idpiNotice,omitempty"`
}

type CaptureEpoch struct {
	FrameID  string `json:"frameId"`
	LoaderID string `json:"loaderId"`
	DomEpoch string `json:"domEpoch"`
}

type CapturePairing struct {
	Navigated         bool  `json:"navigated"`
	CaptureDurationMs int64 `json:"captureDurationMs"`
}

// CaptureImage carries path OR base64, never both: output=file writes the image
// and names the path, output=inline returns the bytes.
type CaptureImage struct {
	Format           string          `json:"format"`
	Bytes            int             `json:"bytes"`
	CoordinateSpace  string          `json:"coordinateSpace"`
	DevicePixelRatio float64         `json:"devicePixelRatio"`
	Viewport         CaptureViewport `json:"viewport"`
	Clip             *CaptureRect    `json:"clip,omitempty"`
	Path             string          `json:"path,omitempty"`
	Base64           string          `json:"base64,omitempty"`
}

type CaptureViewport struct {
	W       float64 `json:"w"`
	H       float64 `json:"h"`
	ScrollX float64 `json:"scrollX"`
	ScrollY float64 `json:"scrollY"`
}

type CaptureRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type CaptureSnapshot struct {
	Filter    string        `json:"filter"`
	NodeCount int           `json:"nodeCount"`
	Nodes     []CaptureNode `json:"nodes"`
}

// CaptureFrame is the scope disclosure the shared frame owner attaches to a
// scoped read. Absent on a whole-document capture.
type CaptureFrame struct {
	FrameID    string `json:"frameId,omitempty"`
	FrameURL   string `json:"frameUrl,omitempty"`
	FrameName  string `json:"frameName,omitempty"`
	OwnerRef   string `json:"ownerRef,omitempty"`
	FrameTitle string `json:"frameTitle,omitempty"`
}

// CaptureNode is the hand-maintained wire twin of observe.A11yNode.
type CaptureNode struct {
	Ref            string              `json:"ref"`
	Role           string              `json:"role"`
	Name           string              `json:"name"`
	Depth          int                 `json:"depth"`
	Value          string              `json:"value,omitempty"`
	Label          string              `json:"label,omitempty"`
	Placeholder    string              `json:"placeholder,omitempty"`
	Alt            string              `json:"alt,omitempty"`
	Title          string              `json:"title,omitempty"`
	TestID         string              `json:"testid,omitempty"`
	Text           string              `json:"text,omitempty"`
	Tag            string              `json:"tag,omitempty"`
	Disabled       bool                `json:"disabled,omitempty"`
	Focused        bool                `json:"focused,omitempty"`
	Checked        string              `json:"checked,omitempty"`
	Hidden         bool                `json:"hidden,omitempty"`
	NodeID         int64               `json:"nodeId,omitempty"`
	FrameID        string              `json:"frameId,omitempty"`
	FrameURL       string              `json:"frameUrl,omitempty"`
	FrameName      string              `json:"frameName,omitempty"`
	ChildFrameID   string              `json:"childFrameId,omitempty"`
	ChildFrameURL  string              `json:"childFrameUrl,omitempty"`
	ChildFrameName string              `json:"childFrameName,omitempty"`
	BoundingBox    *CaptureBoundingBox `json:"boundingBox,omitempty"`
	Visible        *bool               `json:"visible,omitempty"`
}

type CaptureBoundingBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}
