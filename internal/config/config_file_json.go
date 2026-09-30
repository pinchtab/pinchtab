package config

type fileConfigJSON struct {
	Schema           string                      `json:"$schema,omitempty"`
	ConfigVersion    string                      `json:"configVersion,omitempty"`
	Server           serverConfigJSON            `json:"server"`
	Browser          browserConfigJSON           `json:"browser"`
	Browsers         *BrowsersConfig             `json:"browsers,omitempty"`
	InstanceDefaults instanceDefaultsConfigJSON  `json:"instanceDefaults"`
	Security         securityConfigJSON          `json:"security"`
	Profiles         profilesConfigJSON          `json:"profiles"`
	MultiInstance    multiInstanceConfigJSON     `json:"multiInstance"`
	Timeouts         timeoutsConfigJSON          `json:"timeouts"`
	Scheduler        schedulerFileConfigJSON     `json:"scheduler"`
	Observability    observabilityFileConfigJSON `json:"observability"`
	Sessions         sessionsFileConfigJSON      `json:"sessions"`
	AutoSolver       autoSolverFileConfigJSON    `json:"autoSolver,omitempty"`
}

type serverConfigJSON struct {
	Port                      string `json:"port"`
	Bind                      string `json:"bind"`
	Token                     string `json:"token"`
	StateDir                  string `json:"stateDir"`
	LogLevel                  string `json:"logLevel,omitempty"`
	NetworkBufferSize         *int   `json:"networkBufferSize,omitempty"`
	RetainNetworkBodies       *bool  `json:"retainNetworkBodies,omitempty"`
	RetainNetworkBodyMaxBytes *int   `json:"retainNetworkBodyMaxBytes,omitempty"`
	TrustProxyHeaders         *bool  `json:"trustProxyHeaders,omitempty"`
	CookieSecure              *bool  `json:"cookieSecure,omitempty"`
}

type browserConfigJSON struct {
	Provider          string                  `json:"provider,omitempty"`
	BrowserVersion    string                  `json:"version"`
	BrowserBinary     string                  `json:"binary"`
	BrowserDebugPort  *int                    `json:"remoteDebuggingPort,omitempty"`
	BrowserExtraFlags string                  `json:"extraFlags"`
	Cloak             *cloakBrowserConfigJSON `json:"cloak,omitempty"`
	ExtensionPaths    []string                `json:"extensionPaths"`
	Proxy             *BrowserProxyConfig     `json:"proxy,omitempty"`
	DefaultTarget     string                  `json:"defaultTarget,omitempty"`
	FallbackOrder     []string                `json:"fallbackOrder,omitempty"`
	Targets           BrowserTargetsConfig    `json:"targets,omitempty"`
}

type cloakBrowserConfigJSON struct {
	FingerprintSeed           string `json:"fingerprintSeed,omitempty"`
	Platform                  string `json:"platform,omitempty"`
	Locale                    string `json:"locale,omitempty"`
	Timezone                  string `json:"timezone,omitempty"`
	WebRTCIP                  string `json:"webrtcIP,omitempty"`
	FontsDir                  string `json:"fontsDir,omitempty"`
	StorageQuotaMB            *int   `json:"storageQuotaMB,omitempty"`
	DisableDefaultStealthArgs *bool  `json:"disableDefaultStealthArgs,omitempty"`
}

type instanceDefaultsConfigJSON struct {
	Mode                   string             `json:"mode"`
	NoRestore              *bool              `json:"noRestore,omitempty"`
	Timezone               string             `json:"timezone"`
	BlockImages            *bool              `json:"blockImages,omitempty"`
	BlockMedia             *bool              `json:"blockMedia,omitempty"`
	BlockAds               *bool              `json:"blockAds,omitempty"`
	MaxTabs                *int               `json:"maxTabs,omitempty"`
	MaxParallelTabs        *int               `json:"maxParallelTabs,omitempty"`
	UserAgent              string             `json:"userAgent"`
	NoAnimations           *bool              `json:"noAnimations,omitempty"`
	CaptureAllowActivation *bool              `json:"captureAllowActivation,omitempty"`
	Humanize               *bool              `json:"humanize,omitempty"`
	StealthLevel           string             `json:"stealthLevel"`
	TabEvictionPolicy      string             `json:"tabEvictionPolicy"`
	TabPolicy              *TabPolicyDefaults `json:"tabPolicy,omitempty"`
	DialogAutoAccept       *bool              `json:"dialogAutoAccept,omitempty"`
}

type profilesConfigJSON struct {
	BaseDir        string `json:"baseDir"`
	DefaultProfile string `json:"defaultProfile"`
	QuarantineKeep *int   `json:"quarantineKeep,omitempty"`
}

type securityConfigJSON struct {
	AllowEvaluate          *bool          `json:"allowEvaluate,omitempty"`
	AllowMacro             *bool          `json:"allowMacro,omitempty"`
	AllowScreencast        *bool          `json:"allowScreencast,omitempty"`
	AllowDownload          *bool          `json:"allowDownload,omitempty"`
	AllowCookies           *bool          `json:"allowCookies,omitempty"`
	AllowNetworkIntercept  *bool          `json:"allowNetworkIntercept,omitempty"`
	AllowMemory            *bool          `json:"allowMemory,omitempty"`
	AllowFileScheme        *bool          `json:"allowFileScheme,omitempty"`
	AllowedDomains         []string       `json:"allowedDomains,omitzero"`
	DownloadAllowedDomains []string       `json:"downloadAllowedDomains"`
	DownloadMaxBytes       *int           `json:"downloadMaxBytes,omitempty"`
	MemorySnapshotMaxBytes *int           `json:"memorySnapshotMaxBytes,omitempty"`
	AllowUpload            *bool          `json:"allowUpload,omitempty"`
	AllowClipboard         *bool          `json:"allowClipboard,omitempty"`
	AllowStateExport       *bool          `json:"allowStateExport,omitempty"`
	StateEncryptionKey     *string        `json:"stateEncryptionKey,omitempty"`
	UploadMaxRequestBytes  *int           `json:"uploadMaxRequestBytes,omitempty"`
	UploadMaxFiles         *int           `json:"uploadMaxFiles,omitempty"`
	UploadMaxFileBytes     *int           `json:"uploadMaxFileBytes,omitempty"`
	UploadMaxTotalBytes    *int           `json:"uploadMaxTotalBytes,omitempty"`
	MaxRedirects           *int           `json:"maxRedirects,omitempty"`
	TrustedProxyCIDRs      []string       `json:"trustedProxyCIDRs"`
	TrustedResolveCIDRs    []string       `json:"trustedResolveCIDRs"`
	TrustLoopbackProxy     *bool          `json:"trustLoopbackProxy,omitempty"`
	Attach                 attachJSON     `json:"attach"`
	IDPI                   idpiConfigJSON `json:"idpi"`
}

type attachJSON struct {
	Enabled          *bool    `json:"enabled,omitempty"`
	AllowHosts       []string `json:"allowHosts"`
	AllowSchemes     []string `json:"allowSchemes"`
	ForwardProxyAuth *bool    `json:"forwardProxyAuth,omitempty"`
}

type idpiConfigJSON struct {
	Enabled         bool     `json:"enabled"`
	StrictMode      bool     `json:"strictMode"`
	ScanContent     bool     `json:"scanContent"`
	WrapContent     bool     `json:"wrapContent"`
	CustomPatterns  []string `json:"customPatterns"`
	ScanTimeoutSec  int      `json:"scanTimeoutSec"`
	ShieldThreshold int      `json:"shieldThreshold"`
}

type multiInstanceConfigJSON struct {
	Strategy          string                   `json:"strategy"`
	AllocationPolicy  string                   `json:"allocationPolicy"`
	InstancePortStart *int                     `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int                     `json:"instancePortEnd,omitempty"`
	Restart           multiInstanceRestartJSON `json:"restart"`
}

type multiInstanceRestartJSON struct {
	MaxRestarts    *int `json:"maxRestarts,omitempty"`
	InitBackoffSec *int `json:"initBackoffSec,omitempty"`
	MaxBackoffSec  *int `json:"maxBackoffSec,omitempty"`
	StableAfterSec *int `json:"stableAfterSec,omitempty"`
}

type timeoutsConfigJSON struct {
	ActionSec   int `json:"actionSec"`
	NavigateSec int `json:"navigateSec"`
	ShutdownSec int `json:"shutdownSec"`
	WaitNavMs   int `json:"waitNavMs"`
}

type schedulerFileConfigJSON struct {
	Enabled           *bool  `json:"enabled,omitempty"`
	Strategy          string `json:"strategy"`
	MaxQueueSize      *int   `json:"maxQueueSize,omitempty"`
	MaxPerAgent       *int   `json:"maxPerAgent,omitempty"`
	MaxInflight       *int   `json:"maxInflight,omitempty"`
	MaxPerAgentFlight *int   `json:"maxPerAgentInflight,omitempty"`
	ResultTTLSec      *int   `json:"resultTTLSec,omitempty"`
	WorkerCount       *int   `json:"workerCount,omitempty"`
	MaxBatchSize      *int   `json:"maxBatchSize,omitempty"`
}

type observabilityFileConfigJSON struct {
	Activity activityConfigJSON `json:"activity"`
}

type activityConfigJSON struct {
	Enabled        *bool                    `json:"enabled,omitempty"`
	SessionIdleSec *int                     `json:"sessionIdleSec,omitempty"`
	RetentionDays  *int                     `json:"retentionDays,omitempty"`
	StateDir       string                   `json:"stateDir"`
	Events         activityEventsConfigJSON `json:"events"`
}

type activityEventsConfigJSON struct {
	Dashboard    *bool `json:"dashboard,omitempty"`
	Server       *bool `json:"server,omitempty"`
	Bridge       *bool `json:"bridge,omitempty"`
	Orchestrator *bool `json:"orchestrator,omitempty"`
	Scheduler    *bool `json:"scheduler,omitempty"`
	MCP          *bool `json:"mcp,omitempty"`
	Other        *bool `json:"other,omitempty"`
}

type sessionsFileConfigJSON struct {
	Dashboard dashboardSessionConfigJSON `json:"dashboard"`
	Agent     agentSessionConfigJSON     `json:"agent,omitempty"`
}

type agentSessionConfigJSON struct {
	Enabled        *bool  `json:"enabled,omitempty"`
	Mode           string `json:"mode,omitempty"`
	IdleTimeoutSec *int   `json:"idleTimeoutSec,omitempty"`
	MaxLifetimeSec *int   `json:"maxLifetimeSec,omitempty"`
}

type dashboardSessionConfigJSON struct {
	Persist                       *bool `json:"persist,omitempty"`
	IdleTimeoutSec                *int  `json:"idleTimeoutSec,omitempty"`
	MaxLifetimeSec                *int  `json:"maxLifetimeSec,omitempty"`
	ElevationWindowSec            *int  `json:"elevationWindowSec,omitempty"`
	PersistElevationAcrossRestart *bool `json:"persistElevationAcrossRestart,omitempty"`
	RequireElevation              *bool `json:"requireElevation,omitempty"`
}

type autoSolverFileConfigJSON struct {
	Enabled           *bool                           `json:"enabled,omitempty"`
	AutoTrigger       *bool                           `json:"autoTrigger,omitempty"`
	TriggerOnNavigate *bool                           `json:"triggerOnNavigate,omitempty"`
	TriggerOnAction   *bool                           `json:"triggerOnAction,omitempty"`
	MaxAttempts       *int                            `json:"maxAttempts,omitempty"`
	SolverTimeoutSec  *int                            `json:"solverTimeoutSec,omitempty"`
	RetryBaseDelayMs  *int                            `json:"retryBaseDelayMs,omitempty"`
	RetryMaxDelayMs   *int                            `json:"retryMaxDelayMs,omitempty"`
	Solvers           []string                        `json:"solvers,omitempty"`
	LLMProvider       string                          `json:"llmProvider,omitempty"`
	LLMFallback       *bool                           `json:"llmFallback,omitempty"`
	External          autoSolverExtConfigJSON         `json:"external,omitempty"`
	Credentials       autoSolverCredentialsConfigJSON `json:"credentials,omitempty"`
}

type autoSolverExtConfigJSON struct {
	CapsolverKey  string `json:"capsolverKey,omitempty"`
	TwoCaptchaKey string `json:"twoCaptchaKey,omitempty"`
}

type autoSolverCredentialsConfigJSON struct {
	Login  autoSolverLoginConfigJSON  `json:"login,omitempty"`
	Signup autoSolverSignupConfigJSON `json:"signup,omitempty"`
	Form   autoSolverFormConfigJSON   `json:"form,omitempty"`
}

type autoSolverLoginConfigJSON struct {
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type autoSolverSignupConfigJSON struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

type autoSolverFormConfigJSON struct {
	Field1 string `json:"field1,omitempty"`
	Field2 string `json:"field2,omitempty"`
	Email  string `json:"email,omitempty"`
}
