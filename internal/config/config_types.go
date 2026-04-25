package config

import "time"

const defaultPort = "9867"

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
	TrustProxyHeaders bool  // Only trust X-Forwarded-*/Forwarded headers when behind a trusted reverse proxy
	CookieSecure      *bool // Nil = auto-detect based on request scheme/host for backward compatibility

	// Security settings
	AllowEvaluate   bool
	AllowMacro      bool
	AllowScreencast bool
	AllowDownload   bool
	// AllowedDomains is the unified per-instance allowlist sourced from
	// security.allowedDomains in the file config.
	AllowedDomains         []string
	DownloadAllowedDomains []string
	DownloadMaxBytes       int
	AllowUpload            bool
	AllowClipboard         bool
	AllowStateExport       bool
	StateEncryptionKey     string // Key for encrypting state files (AES-256-GCM)
	EnableActionGuards     bool   // Enable bridge-level stale/navigation guard checks around actions
	UploadMaxRequestBytes  int
	UploadMaxFiles         int
	UploadMaxFileBytes     int
	UploadMaxTotalBytes    int
	MaxRedirects           int      // Max HTTP redirects (-1=unlimited, 0=none, default=-1)
	TrustedProxyCIDRs      []string // CIDRs/IPs whose RemoteIPAddress is trusted in navigation responses (e.g. internal proxy)
	TrustedResolveCIDRs    []string // CIDRs/IPs allowed when a navigation target resolves to non-public addresses

	// Browser/instance settings
	Headless           bool
	HeadlessSet        bool // true when explicitly set via config or flag
	NoRestore          bool
	ProfileDir         string
	ProfilesBaseDir    string
	DefaultProfile     string
	ChromeVersion      string
	Timezone           string
	BlockImages        bool
	BlockMedia         bool
	BlockAds           bool
	MaxTabs            int
	MaxParallelTabs    int // 0 = auto-detect from runtime.NumCPU
	ChromeBinary       string
	ChromeDebugPort    int
	ChromeExtraFlags   string
	ExtensionPaths     []string
	UserAgent          string
	NoAnimations       bool
	StealthLevel       string
	TabEvictionPolicy  string        // "close_lru" (default), "reject", "close_oldest" — fires on MaxTabs pressure
	TabLifecyclePolicy string        // "close_idle" (default), "keep" — fires on idle after read/action
	TabCloseDelay      time.Duration // applies when TabLifecyclePolicy == "close_idle" (default 5m)
	TabRestore         bool          // restore previously open tabs from sessions.json on startup (default false)

	// Timeout settings
	ActionTimeout   time.Duration
	NavigateTimeout time.Duration
	ShutdownTimeout time.Duration
	WaitNavDelay    time.Duration

	// Orchestrator settings (dashboard mode only)
	Strategy           string        // "always-on" (default), "simple", "explicit", or "simple-autorestart"
	AllocationPolicy   string        // "fcfs" (default), "round_robin", "random"
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

	// Session settings
	Sessions SessionsRuntimeConfig

	// AutoSolver settings
	AutoSolver AutoSolverConfig
}

type SessionsRuntimeConfig struct {
	Dashboard DashboardSessionRuntimeConfig `json:"dashboard,omitempty"`
	Agent     AgentSessionRuntimeConfig     `json:"agent,omitempty"`
}

type AgentSessionRuntimeConfig struct {
	Enabled     bool          `json:"enabled,omitempty"`
	Mode        string        `json:"mode,omitempty"`
	IdleTimeout time.Duration `json:"idleTimeout,omitempty"`
	MaxLifetime time.Duration `json:"maxLifetime,omitempty"`
}

type DashboardSessionRuntimeConfig struct {
	Persist                       bool          `json:"persist,omitempty"`
	IdleTimeout                   time.Duration `json:"idleTimeout,omitempty"`
	MaxLifetime                   time.Duration `json:"maxLifetime,omitempty"`
	ElevationWindow               time.Duration `json:"elevationWindow,omitempty"`
	PersistElevationAcrossRestart bool          `json:"persistElevationAcrossRestart,omitempty"`
	RequireElevation              bool          `json:"requireElevation,omitempty"`
}

// IDPIConfig holds the configuration for the Indirect Prompt Injection (IDPI)
// defense layer.
type IDPIConfig struct {
	Enabled        bool     `json:"enabled,omitempty"`
	StrictMode     bool     `json:"strictMode,omitempty"`
	ScanContent    bool     `json:"scanContent,omitempty"`
	WrapContent    bool     `json:"wrapContent,omitempty"`
	CustomPatterns []string `json:"customPatterns,omitempty"`
	ScanTimeoutSec int      `json:"scanTimeoutSec,omitempty"`
	// ShieldThreshold sets the minimum score (0-100) from idpishield
	// to flag content as a threat. Lower = more sensitive.
	// When zero, idpishield defaults apply (40 strict, 60 normal).
	ShieldThreshold int `json:"shieldThreshold,omitempty"`
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

// AutoSolverConfig holds autosolver runtime settings.
type AutoSolverConfig struct {
	Enabled           bool     `json:"enabled,omitempty"`
	AutoTrigger       bool     `json:"autoTrigger,omitempty"`
	TriggerOnNavigate bool     `json:"triggerOnNavigate,omitempty"`
	TriggerOnAction   bool     `json:"triggerOnAction,omitempty"`
	MaxAttempts       int      `json:"maxAttempts,omitempty"`
	SolverTimeoutSec  int      `json:"solverTimeoutSec,omitempty"`
	RetryBaseDelayMs  int      `json:"retryBaseDelayMs,omitempty"`
	RetryMaxDelayMs   int      `json:"retryMaxDelayMs,omitempty"`
	Solvers           []string `json:"solvers,omitempty"`     // Ordered solver names
	LLMProvider       string   `json:"llmProvider,omitempty"` // "openai", "anthropic", etc.
	LLMFallback       bool     `json:"llmFallback,omitempty"` // Enable LLM as last resort
	CapsolverKey      string   `json:"capsolverKey,omitempty"`
	TwoCaptchaKey     string   `json:"twoCaptchaKey,omitempty"`
	Credentials       AutoSolverCredentials
}

// AutoSolverCredentials carries values the semantic solver injects into
// matched login/signup/form fields. Persisted to the config file but
// redacted when read back through the dashboard config API.
type AutoSolverCredentials struct {
	Login  AutoSolverLoginCreds
	Signup AutoSolverSignupCreds
	Form   AutoSolverFormCreds
}

type AutoSolverLoginCreds struct {
	User     string
	Password string
}

type AutoSolverSignupCreds struct {
	Name     string
	Email    string
	Password string
}

type AutoSolverFormCreds struct {
	Field1 string
	Field2 string
	Email  string
}

type ObservabilityConfig struct {
	Activity ActivityConfig `json:"activity,omitempty"`
}

type ActivityConfig struct {
	Enabled        bool                 `json:"enabled,omitempty"`
	SessionIdleSec int                  `json:"sessionIdleSec,omitempty"`
	RetentionDays  int                  `json:"retentionDays,omitempty"`
	StateDir       string               `json:"stateDir,omitempty"`
	Events         ActivityEventsConfig `json:"events,omitempty"`
}

type ActivityEventsConfig struct {
	Dashboard    bool `json:"dashboard,omitempty"`
	Server       bool `json:"server,omitempty"`
	Bridge       bool `json:"bridge,omitempty"`
	Orchestrator bool `json:"orchestrator,omitempty"`
	Scheduler    bool `json:"scheduler,omitempty"`
	MCP          bool `json:"mcp,omitempty"`
	Other        bool `json:"other,omitempty"`
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
	Sessions         SessionsFileConfig      `json:"sessions,omitempty"`
	AutoSolver       AutoSolverFileConfig    `json:"autoSolver,omitempty"`
}

type ServerConfig struct {
	Port              string `json:"port,omitempty"`
	Bind              string `json:"bind,omitempty"`
	Token             string `json:"token,omitempty"`
	StateDir          string `json:"stateDir,omitempty"`
	Engine            string `json:"engine,omitempty"`
	NetworkBufferSize *int   `json:"networkBufferSize,omitempty"`
	TrustProxyHeaders *bool  `json:"trustProxyHeaders,omitempty"`
	CookieSecure      *bool  `json:"cookieSecure,omitempty"`
}

type SessionsFileConfig struct {
	Dashboard DashboardSessionFileConfig `json:"dashboard,omitempty"`
	Agent     AgentSessionFileConfig     `json:"agent,omitempty"`
}

type AgentSessionFileConfig struct {
	Enabled        *bool  `json:"enabled,omitempty"`
	Mode           string `json:"mode,omitempty"`
	IdleTimeoutSec *int   `json:"idleTimeoutSec,omitempty"`
	MaxLifetimeSec *int   `json:"maxLifetimeSec,omitempty"`
}

type DashboardSessionFileConfig struct {
	Persist                       *bool `json:"persist,omitempty"`
	IdleTimeoutSec                *int  `json:"idleTimeoutSec,omitempty"`
	MaxLifetimeSec                *int  `json:"maxLifetimeSec,omitempty"`
	ElevationWindowSec            *int  `json:"elevationWindowSec,omitempty"`
	PersistElevationAcrossRestart *bool `json:"persistElevationAcrossRestart,omitempty"`
	RequireElevation              *bool `json:"requireElevation,omitempty"`
}

type BrowserConfig struct {
	ChromeVersion    string   `json:"version,omitempty"`
	ChromeBinary     string   `json:"binary,omitempty"`
	ChromeDebugPort  *int     `json:"remoteDebuggingPort,omitempty"`
	ChromeExtraFlags string   `json:"extraFlags,omitempty"`
	ExtensionPaths   []string `json:"extensionPaths,omitempty"`
}

type InstanceDefaultsConfig struct {
	Mode              string             `json:"mode,omitempty"`
	NoRestore         *bool              `json:"noRestore,omitempty"`
	Timezone          string             `json:"timezone,omitempty"`
	BlockImages       *bool              `json:"blockImages,omitempty"`
	BlockMedia        *bool              `json:"blockMedia,omitempty"`
	BlockAds          *bool              `json:"blockAds,omitempty"`
	MaxTabs           *int               `json:"maxTabs,omitempty"`
	MaxParallelTabs   *int               `json:"maxParallelTabs,omitempty"`
	UserAgent         string             `json:"userAgent,omitempty"`
	NoAnimations      *bool              `json:"noAnimations,omitempty"`
	StealthLevel      string             `json:"stealthLevel,omitempty"`
	TabEvictionPolicy string             `json:"tabEvictionPolicy,omitempty"` // Deprecated: use TabPolicy.Eviction
	TabPolicy         *TabPolicyDefaults `json:"tabPolicy,omitempty"`
	DialogAutoAccept  *bool              `json:"dialogAutoAccept,omitempty"`
}

// TabPolicyDefaults groups eviction (cap pressure) and lifecycle (idle) policies
// in instance-defaults configs. Either sub-field may be omitted.
type TabPolicyDefaults struct {
	Eviction      string `json:"eviction,omitempty"`      // "close_lru" | "reject" | "close_oldest"
	Lifecycle     string `json:"lifecycle,omitempty"`     // "keep" | "close_idle"
	CloseDelaySec *int   `json:"closeDelaySec,omitempty"` // applies to close_idle; default 300
	Restore       *bool  `json:"restore,omitempty"`       // restore tabs from sessions.json on startup; default false
}

type ProfilesConfig struct {
	BaseDir        string `json:"baseDir,omitempty"`
	DefaultProfile string `json:"defaultProfile,omitempty"`
}

type SecurityConfig struct {
	AllowEvaluate          *bool        `json:"allowEvaluate,omitempty"`
	AllowMacro             *bool        `json:"allowMacro,omitempty"`
	AllowScreencast        *bool        `json:"allowScreencast,omitempty"`
	AllowDownload          *bool        `json:"allowDownload,omitempty"`
	AllowedDomains         []string     `json:"allowedDomains,omitempty"`
	DownloadAllowedDomains []string     `json:"downloadAllowedDomains,omitempty"`
	DownloadMaxBytes       *int         `json:"downloadMaxBytes,omitempty"`
	AllowUpload            *bool        `json:"allowUpload,omitempty"`
	AllowClipboard         *bool        `json:"allowClipboard,omitempty"`
	AllowStateExport       *bool        `json:"allowStateExport,omitempty"`
	StateEncryptionKey     *string      `json:"stateEncryptionKey,omitempty"`
	EnableActionGuards     *bool        `json:"enableActionGuards,omitempty"`
	UploadMaxRequestBytes  *int         `json:"uploadMaxRequestBytes,omitempty"`
	UploadMaxFiles         *int         `json:"uploadMaxFiles,omitempty"`
	UploadMaxFileBytes     *int         `json:"uploadMaxFileBytes,omitempty"`
	UploadMaxTotalBytes    *int         `json:"uploadMaxTotalBytes,omitempty"`
	MaxRedirects           *int         `json:"maxRedirects,omitempty"`
	TrustedProxyCIDRs      []string     `json:"trustedProxyCIDRs,omitempty"`
	TrustedResolveCIDRs    []string     `json:"trustedResolveCIDRs,omitempty"`
	Attach                 AttachConfig `json:"attach,omitempty"`
	IDPI                   IDPIConfig   `json:"idpi,omitempty"`
}

type MultiInstanceConfig struct {
	Strategy          string                     `json:"strategy,omitempty"`
	AllocationPolicy  string                     `json:"allocationPolicy,omitempty"`
	InstancePortStart *int                       `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int                       `json:"instancePortEnd,omitempty"`
	Restart           MultiInstanceRestartConfig `json:"restart,omitempty"`
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
	Enabled        *bool                    `json:"enabled,omitempty"`
	SessionIdleSec *int                     `json:"sessionIdleSec,omitempty"`
	RetentionDays  *int                     `json:"retentionDays,omitempty"`
	StateDir       string                   `json:"stateDir,omitempty"`
	Events         ActivityEventsFileConfig `json:"events,omitempty"`
}

type ActivityEventsFileConfig struct {
	Dashboard    *bool `json:"dashboard,omitempty"`
	Server       *bool `json:"server,omitempty"`
	Bridge       *bool `json:"bridge,omitempty"`
	Orchestrator *bool `json:"orchestrator,omitempty"`
	Scheduler    *bool `json:"scheduler,omitempty"`
	MCP          *bool `json:"mcp,omitempty"`
	Other        *bool `json:"other,omitempty"`
}

// AutoSolverFileConfig is the persistent configuration for the autosolver system.
type AutoSolverFileConfig struct {
	Enabled           *bool                     `json:"enabled,omitempty"`
	AutoTrigger       *bool                     `json:"autoTrigger,omitempty"`
	TriggerOnNavigate *bool                     `json:"triggerOnNavigate,omitempty"`
	TriggerOnAction   *bool                     `json:"triggerOnAction,omitempty"`
	MaxAttempts       *int                      `json:"maxAttempts,omitempty"`
	SolverTimeoutSec  *int                      `json:"solverTimeoutSec,omitempty"`
	RetryBaseDelayMs  *int                      `json:"retryBaseDelayMs,omitempty"`
	RetryMaxDelayMs   *int                      `json:"retryMaxDelayMs,omitempty"`
	Solvers           []string                  `json:"solvers,omitempty"`
	LLMProvider       string                    `json:"llmProvider,omitempty"`
	LLMFallback       *bool                     `json:"llmFallback,omitempty"`
	External          AutoSolverExtConf         `json:"external,omitempty"`
	Credentials       AutoSolverCredentialsConf `json:"credentials,omitempty"`
}

// AutoSolverExtConf holds external solver API keys.
type AutoSolverExtConf struct {
	CapsolverKey  string `json:"capsolverKey,omitempty"`
	TwoCaptchaKey string `json:"twoCaptchaKey,omitempty"`
}

// AutoSolverCredentialsConf is the persisted form of the credentials block.
// All fields are write-only from the dashboard's perspective: GET /api/config
// returns them blanked, PUT preserves the on-disk values when blank.
type AutoSolverCredentialsConf struct {
	Login  AutoSolverLoginConf  `json:"login,omitempty"`
	Signup AutoSolverSignupConf `json:"signup,omitempty"`
	Form   AutoSolverFormConf   `json:"form,omitempty"`
}

type AutoSolverLoginConf struct {
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type AutoSolverSignupConf struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

type AutoSolverFormConf struct {
	Field1 string `json:"field1,omitempty"`
	Field2 string `json:"field2,omitempty"`
	Email  string `json:"email,omitempty"`
}
