// Package types contains shared API types for the dashboard.
// These types are exported to TypeScript via tygo.
package types

import "time"

// Profile represents a browser profile stored on disk.
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
	Source            string    `json:"source,omitempty"`
	ChromeProfileName string    `json:"chromeProfileName,omitempty"`
	AccountEmail      string    `json:"accountEmail,omitempty"`
	AccountName       string    `json:"accountName,omitempty"`
	HasAccount        bool      `json:"hasAccount,omitempty"`
	UseWhen           string    `json:"useWhen,omitempty"`
	Description       string    `json:"description,omitempty"`
}

// Instance represents a running browser instance.
// Matches internal/bridge/api.go Instance
type Instance struct {
	ID          string    `json:"id"`
	ProfileID   string    `json:"profileId"`
	ProfileName string    `json:"profileName"`
	Port        string    `json:"port"` // Note: string not int
	Headless    bool      `json:"headless"`
	Status      string    `json:"status"` // starting/running/stopping/stopped/error
	StartTime   time.Time `json:"startTime"`
	Error       string    `json:"error,omitempty"`
}

// Agent represents a connected AI agent.
type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	ConnectedAt  time.Time `json:"connectedAt"`
	LastActivity time.Time `json:"lastActivity,omitempty"`
	RequestCount int       `json:"requestCount"`
}

// ActivityEvent represents an action in the activity feed.
type ActivityEvent struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agentId"`
	Type      string                 `json:"type"` // navigate/snapshot/action/screenshot/other
	Method    string                 `json:"method"`
	Path      string                 `json:"path"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ScreencastSettings configures live tab previews.
type ScreencastSettings struct {
	FPS      int `json:"fps"`
	Quality  int `json:"quality"`
	MaxWidth int `json:"maxWidth"`
}

// BrowserSettings configures browser behavior.
type BrowserSettings struct {
	BlockImages  bool `json:"blockImages"`
	BlockMedia   bool `json:"blockMedia"`
	NoAnimations bool `json:"noAnimations"`
}

// Settings contains all dashboard settings.
type Settings struct {
	Screencast ScreencastSettings `json:"screencast"`
	Stealth    string             `json:"stealth"` // light/full
	Browser    BrowserSettings    `json:"browser"`
}

// ServerInfo contains health/status information.
type ServerInfo struct {
	Version   string `json:"version"`
	Uptime    int64  `json:"uptime"`
	Profiles  int    `json:"profiles"`
	Instances int    `json:"instances"`
	Agents    int    `json:"agents"`
}

// CreateProfileRequest is the request body for creating a profile.
type CreateProfileRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	UseWhen     string `json:"useWhen,omitempty"`
}

// CreateProfileResponse is returned after creating a profile.
type CreateProfileResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

// InstanceTab represents a browser tab in an instance.
type InstanceTab struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// LaunchInstanceRequest is the request body for launching an instance.
type LaunchInstanceRequest struct {
	ProfileID string `json:"profileId,omitempty"` // profile ID (prof_XXXXXXXX)
	Name      string `json:"name,omitempty"`      // profile name
	Mode      string `json:"mode,omitempty"`      // "headed" or empty for headless
	Port      string `json:"port,omitempty"`      // port number as string
}
