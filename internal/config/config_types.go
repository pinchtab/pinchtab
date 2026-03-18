package config

import "time"

// RuntimeConfig holds all runtime settings used throughout the application.
// This is the single source of truth for configuration at runtime.
type RuntimeConfig struct {
	// Server settings
	Bind              string
	Port              string
	InstancePortStart int // Starting port for instances (default 9868)
	InstancePortEnd   int // Ending port for instances (default 9968)
	Token             string
	StateDir          string

	// Security settings
	AllowEvaluate   bool
	AllowMacro      bool
	AllowScreencast bool
	AllowDownload   bool
	AllowUpload     bool
	MaxRedirects    int // Max HTTP redirects (-1=unlimited, 0=none, default=-1)

	// Browser/instance settings
	Headless          bool
	NoRestore         bool
	ProfileDir        string
	ProfilesBaseDir   string
	DefaultProfile    string
	ChromeVersion     string
	Timezone          string
	BlockImages       bool
	BlockMedia        bool
	BlockAds          bool
	MaxTabs           int
	MaxParallelTabs   int // 0 = auto-detect from runtime.NumCPU
	ChromeBinary      string
	ChromeExtraFlags  string
	ExtensionPaths    []string
	UserAgent         string
	NoAnimations      bool
	StealthLevel      string
	TabEvictionPolicy string // "close_lru" (default), "reject", "close_oldest"

	// Timeout settings
	ActionTimeout   time.Duration
	NavigateTimeout time.Duration
	ShutdownTimeout time.Duration
	WaitNavDelay    time.Duration

	// Orchestrator settings (dashboard mode only)
	Strategy           string // "always-on" (default), "simple", "explicit", or "simple-autorestart"
	AllocationPolicy   string // "fcfs" (default), "round_robin", "random"
	DisableLocalLaunch bool
	RestartMaxRestarts int           // Max restart attempts for restart-managed strategies (-1 = unlimited, 0 = strategy default)
	RestartInitBackoff time.Duration // Initial restart backoff (0 = strategy default)
	RestartMaxBackoff  time.Duration // Maximum restart backoff cap (0 = strategy default)
	RestartStableAfter time.Duration // Stable runtime window that resets the restart counter (0 = strategy default)

	// Attach settings
	AttachEnabled      bool
	AttachAllowHosts   []string
	AttachAllowSchemes []string

	// IDPI (Indirect Prompt Injection defense) settings
	IDPI IDPIConfig

	// Dialog settings
	DialogAutoAccept bool

	// Engine mode: "chrome" (default), "lite", or "auto"
	Engine string

	// Network monitoring
	NetworkBufferSize int // Per-tab network buffer size (default 100)

	// Scheduler settings (dashboard mode only)
	Scheduler SchedulerConfig

	// Observability settings
	Observability ObservabilityConfig
}

// IDPIConfig holds the configuration for the Indirect Prompt Injection (IDPI)
// defense layer.
type IDPIConfig struct {
	Enabled        bool     `json:"enabled,omitempty"`
	AllowedDomains []string `json:"allowedDomains,omitempty"`
	StrictMode     bool     `json:"strictMode,omitempty"`
	ScanContent    bool     `json:"scanContent,omitempty"`
	WrapContent    bool     `json:"wrapContent,omitempty"`
	CustomPatterns []string `json:"customPatterns,omitempty"`
	ScanTimeoutSec int      `json:"scanTimeoutSec,omitempty"`
}

// SchedulerConfig holds task scheduler settings.
type SchedulerConfig struct {
	Enabled           bool   `json:"enabled,omitempty"`
	Strategy          string `json:"strategy,omitempty"`
	MaxQueueSize      int    `json:"maxQueueSize,omitempty"`
	MaxPerAgent       int    `json:"maxPerAgent,omitempty"`
	MaxInflight       int    `json:"maxInflight,omitempty"`
	MaxPerAgentFlight int    `json:"maxPerAgentInflight,omitempty"`
	ResultTTLSec      int    `json:"resultTTLSec,omitempty"`
	WorkerCount       int    `json:"workerCount,omitempty"`
}

type ObservabilityConfig struct {
	Activity ActivityConfig `json:"activity,omitempty"`
}

type ActivityConfig struct {
	Enabled        bool `json:"enabled,omitempty"`
	SessionIdleSec int  `json:"sessionIdleSec,omitempty"`
	RetentionDays  int  `json:"retentionDays,omitempty"`
}

// FileConfig is the persistent configuration written to disk.
type FileConfig struct {
	ConfigVersion    string                  `json:"configVersion,omitempty"`
	Server           ServerConfig            `json:"server,omitempty"`
	Browser          BrowserConfig           `json:"browser,omitempty"`
	InstanceDefaults InstanceDefaultsConfig  `json:"instanceDefaults,omitempty"`
	Security         SecurityConfig          `json:"security,omitempty"`
	Profiles         ProfilesConfig          `json:"profiles,omitempty"`
	MultiInstance    MultiInstanceConfig     `json:"multiInstance,omitempty"`
	Timeouts         TimeoutsConfig          `json:"timeouts,omitempty"`
	Scheduler        SchedulerFileConfig     `json:"scheduler,omitempty"`
	Observability    ObservabilityFileConfig `json:"observability,omitempty"`
}

type ServerConfig struct {
	Port              string `json:"port,omitempty"`
	Bind              string `json:"bind,omitempty"`
	Token             string `json:"token,omitempty"`
	StateDir          string `json:"stateDir,omitempty"`
	Engine            string `json:"engine,omitempty"`
	NetworkBufferSize *int   `json:"networkBufferSize,omitempty"`
}

type BrowserConfig struct {
	ChromeVersion    string   `json:"version,omitempty"`
	ChromeBinary     string   `json:"binary,omitempty"`
	ChromeExtraFlags string   `json:"extraFlags,omitempty"`
	ExtensionPaths   []string `json:"extensionPaths,omitempty"`
}

type InstanceDefaultsConfig struct {
	Mode              string `json:"mode,omitempty"`
	NoRestore         *bool  `json:"noRestore,omitempty"`
	Timezone          string `json:"timezone,omitempty"`
	BlockImages       *bool  `json:"blockImages,omitempty"`
	BlockMedia        *bool  `json:"blockMedia,omitempty"`
	BlockAds          *bool  `json:"blockAds,omitempty"`
	MaxTabs           *int   `json:"maxTabs,omitempty"`
	MaxParallelTabs   *int   `json:"maxParallelTabs,omitempty"`
	UserAgent         string `json:"userAgent,omitempty"`
	NoAnimations      *bool  `json:"noAnimations,omitempty"`
	StealthLevel      string `json:"stealthLevel,omitempty"`
	TabEvictionPolicy string `json:"tabEvictionPolicy,omitempty"`
	DialogAutoAccept  *bool  `json:"dialogAutoAccept,omitempty"`
}

type ProfilesConfig struct {
	BaseDir        string `json:"baseDir,omitempty"`
	DefaultProfile string `json:"defaultProfile,omitempty"`
}

type SecurityConfig struct {
	AllowEvaluate   *bool        `json:"allowEvaluate,omitempty"`
	AllowMacro      *bool        `json:"allowMacro,omitempty"`
	AllowScreencast *bool        `json:"allowScreencast,omitempty"`
	AllowDownload   *bool        `json:"allowDownload,omitempty"`
	AllowUpload     *bool        `json:"allowUpload,omitempty"`
	MaxRedirects    *int         `json:"maxRedirects,omitempty"`
	Attach          AttachConfig `json:"attach,omitempty"`
	IDPI            IDPIConfig   `json:"idpi,omitempty"`
}

type MultiInstanceConfig struct {
	Strategy           string                     `json:"strategy,omitempty"`
	AllocationPolicy   string                     `json:"allocationPolicy,omitempty"`
	DisableLocalLaunch *bool                      `json:"disableLocalLaunch,omitempty"`
	InstancePortStart  *int                       `json:"instancePortStart,omitempty"`
	InstancePortEnd    *int                       `json:"instancePortEnd,omitempty"`
	Restart            MultiInstanceRestartConfig `json:"restart,omitempty"`
}

// MultiInstanceRestartConfig controls restart-managed strategy recovery behavior.
type MultiInstanceRestartConfig struct {
	MaxRestarts    *int `json:"maxRestarts,omitempty"`
	InitBackoffSec *int `json:"initBackoffSec,omitempty"`
	MaxBackoffSec  *int `json:"maxBackoffSec,omitempty"`
	StableAfterSec *int `json:"stableAfterSec,omitempty"`
}

type AttachConfig struct {
	Enabled      *bool    `json:"enabled,omitempty"`
	AllowHosts   []string `json:"allowHosts,omitempty"`
	AllowSchemes []string `json:"allowSchemes,omitempty"`
}

type TimeoutsConfig struct {
	ActionSec   int `json:"actionSec,omitempty"`
	NavigateSec int `json:"navigateSec,omitempty"`
	ShutdownSec int `json:"shutdownSec,omitempty"`
	WaitNavMs   int `json:"waitNavMs,omitempty"`
}

type SchedulerFileConfig struct {
	Enabled           *bool  `json:"enabled,omitempty"`
	Strategy          string `json:"strategy,omitempty"`
	MaxQueueSize      *int   `json:"maxQueueSize,omitempty"`
	MaxPerAgent       *int   `json:"maxPerAgent,omitempty"`
	MaxInflight       *int   `json:"maxInflight,omitempty"`
	MaxPerAgentFlight *int   `json:"maxPerAgentInflight,omitempty"`
	ResultTTLSec      *int   `json:"resultTTLSec,omitempty"`
	WorkerCount       *int   `json:"workerCount,omitempty"`
}

type ObservabilityFileConfig struct {
	Activity ActivityFileConfig `json:"activity,omitempty"`
}

type ActivityFileConfig struct {
	Enabled        *bool `json:"enabled,omitempty"`
	SessionIdleSec *int  `json:"sessionIdleSec,omitempty"`
	RetentionDays  *int  `json:"retentionDays,omitempty"`
}
