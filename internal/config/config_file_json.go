package config

type fileConfigJSON struct {
	ConfigVersion    string                      `json:"configVersion,omitempty"`
	Server           serverConfigJSON            `json:"server"`
	Browser          browserConfigJSON           `json:"browser"`
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
	Port              string `json:"port"`
	Bind              string `json:"bind"`
	Token             string `json:"token"`
	StateDir          string `json:"stateDir"`
	Engine            string `json:"engine"`
	NetworkBufferSize *int   `json:"networkBufferSize,omitempty"`
	TrustProxyHeaders *bool  `json:"trustProxyHeaders,omitempty"`
	CookieSecure      *bool  `json:"cookieSecure,omitempty"`
}

type browserConfigJSON struct {
	ChromeVersion    string   `json:"version"`
	ChromeBinary     string   `json:"binary"`
	ChromeDebugPort  *int     `json:"remoteDebuggingPort,omitempty"`
	ChromeExtraFlags string   `json:"extraFlags"`
	ExtensionPaths   []string `json:"extensionPaths"`
}

type instanceDefaultsConfigJSON struct {
	Mode              string             `json:"mode"`
	NoRestore         *bool              `json:"noRestore"`
	Timezone          string             `json:"timezone"`
	BlockImages       *bool              `json:"blockImages"`
	BlockMedia        *bool              `json:"blockMedia"`
	BlockAds          *bool              `json:"blockAds"`
	MaxTabs           *int               `json:"maxTabs"`
	MaxParallelTabs   *int               `json:"maxParallelTabs"`
	UserAgent         string             `json:"userAgent"`
	NoAnimations      *bool              `json:"noAnimations"`
	StealthLevel      string             `json:"stealthLevel"`
	TabEvictionPolicy string             `json:"tabEvictionPolicy"`
	TabPolicy         *TabPolicyDefaults `json:"tabPolicy,omitempty"`
}

type profilesConfigJSON struct {
	BaseDir        string `json:"baseDir"`
	DefaultProfile string `json:"defaultProfile"`
}

type securityConfigJSON struct {
	AllowEvaluate          *bool          `json:"allowEvaluate"`
	AllowMacro             *bool          `json:"allowMacro"`
	AllowScreencast        *bool          `json:"allowScreencast"`
	AllowDownload          *bool          `json:"allowDownload"`
	AllowedDomains         []string       `json:"allowedDomains"`
	DownloadAllowedDomains []string       `json:"downloadAllowedDomains"`
	DownloadMaxBytes       *int           `json:"downloadMaxBytes"`
	AllowUpload            *bool          `json:"allowUpload"`
	AllowClipboard         *bool          `json:"allowClipboard"`
	AllowStateExport       *bool          `json:"allowStateExport"`
	StateEncryptionKey     *string        `json:"stateEncryptionKey"`
	EnableActionGuards     *bool          `json:"enableActionGuards"`
	UploadMaxRequestBytes  *int           `json:"uploadMaxRequestBytes"`
	UploadMaxFiles         *int           `json:"uploadMaxFiles"`
	UploadMaxFileBytes     *int           `json:"uploadMaxFileBytes"`
	UploadMaxTotalBytes    *int           `json:"uploadMaxTotalBytes"`
	MaxRedirects           *int           `json:"maxRedirects"`
	TrustedProxyCIDRs      []string       `json:"trustedProxyCIDRs"`
	TrustedResolveCIDRs    []string       `json:"trustedResolveCIDRs"`
	Attach                 attachJSON     `json:"attach"`
	IDPI                   idpiConfigJSON `json:"idpi"`
}

type attachJSON struct {
	Enabled      *bool    `json:"enabled"`
	AllowHosts   []string `json:"allowHosts"`
	AllowSchemes []string `json:"allowSchemes"`
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
	InstancePortStart *int                     `json:"instancePortStart"`
	InstancePortEnd   *int                     `json:"instancePortEnd"`
	Restart           multiInstanceRestartJSON `json:"restart"`
}

type multiInstanceRestartJSON struct {
	MaxRestarts    *int `json:"maxRestarts"`
	InitBackoffSec *int `json:"initBackoffSec"`
	MaxBackoffSec  *int `json:"maxBackoffSec"`
	StableAfterSec *int `json:"stableAfterSec"`
}

type timeoutsConfigJSON struct {
	ActionSec   int `json:"actionSec"`
	NavigateSec int `json:"navigateSec"`
	ShutdownSec int `json:"shutdownSec"`
	WaitNavMs   int `json:"waitNavMs"`
}

type schedulerFileConfigJSON struct {
	Enabled           *bool  `json:"enabled"`
	Strategy          string `json:"strategy"`
	MaxQueueSize      *int   `json:"maxQueueSize"`
	MaxPerAgent       *int   `json:"maxPerAgent"`
	MaxInflight       *int   `json:"maxInflight"`
	MaxPerAgentFlight *int   `json:"maxPerAgentInflight"`
	ResultTTLSec      *int   `json:"resultTTLSec"`
	WorkerCount       *int   `json:"workerCount"`
}

type observabilityFileConfigJSON struct {
	Activity activityConfigJSON `json:"activity"`
}

type activityConfigJSON struct {
	Enabled        *bool                    `json:"enabled"`
	SessionIdleSec *int                     `json:"sessionIdleSec"`
	RetentionDays  *int                     `json:"retentionDays"`
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
