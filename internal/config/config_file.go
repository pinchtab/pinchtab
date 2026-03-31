package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// CurrentConfigVersion is bumped when config schema changes require migration or wizard re-run.
const CurrentConfigVersion = "0.8.0"

// DefaultFileConfig returns a FileConfig with sensible defaults (nested format).
func DefaultFileConfig() FileConfig {
	start := 9868
	end := 9968
	restartMaxRestarts := 20
	restartInitBackoffSec := 2
	restartMaxBackoffSec := 60
	restartStableAfterSec := 300
	maxTabs := 20
	allowEvaluate := false
	allowMacro := false
	allowScreencast := false
	allowDownload := false
	downloadMaxBytes := DefaultDownloadMaxBytes
	allowUpload := false
	allowClipboard := false
	uploadMaxRequestBytes := DefaultUploadMaxRequestBytes
	uploadMaxFiles := DefaultUploadMaxFiles
	uploadMaxFileBytes := DefaultUploadMaxFileBytes
	uploadMaxTotalBytes := DefaultUploadMaxTotalBytes
	maxRedirects := -1
	attachEnabled := false
	activityEnabled := true
	activitySessionIdleSec := 1800
	activityRetentionDays := 1
	dashboardSessionPersist := true
	dashboardSessionIdleSec := 7 * 24 * 60 * 60
	dashboardSessionMaxLifetimeSec := 7 * 24 * 60 * 60
	dashboardSessionElevationWindowSec := 15 * 60
	dashboardSessionPersistElevationAcrossRestart := false
	dashboardSessionRequireElevation := false
	return FileConfig{
		ConfigVersion: CurrentConfigVersion,
		Server: ServerConfig{
			Port:     "9867",
			Bind:     "127.0.0.1",
			StateDir: userConfigDir(),
		},
		Browser: BrowserConfig{
			ChromeVersion: "144.0.7559.133",
		},
		InstanceDefaults: InstanceDefaultsConfig{
			Mode:              "headless",
			MaxTabs:           &maxTabs,
			StealthLevel:      "light",
			TabEvictionPolicy: "close_lru",
		},
		Security: SecurityConfig{
			AllowEvaluate:          &allowEvaluate,
			AllowMacro:             &allowMacro,
			AllowScreencast:        &allowScreencast,
			AllowDownload:          &allowDownload,
			DownloadAllowedDomains: []string{},
			DownloadMaxBytes:       &downloadMaxBytes,
			AllowUpload:            &allowUpload,
			AllowClipboard:         &allowClipboard,
			UploadMaxRequestBytes:  &uploadMaxRequestBytes,
			UploadMaxFiles:         &uploadMaxFiles,
			UploadMaxFileBytes:     &uploadMaxFileBytes,
			UploadMaxTotalBytes:    &uploadMaxTotalBytes,
			MaxRedirects:           &maxRedirects,
			Attach: AttachConfig{
				Enabled:      &attachEnabled,
				AllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
				AllowSchemes: []string{"ws", "wss"},
			},
			IDPI: IDPIConfig{
				Enabled:        true,
				AllowedDomains: append([]string(nil), defaultLocalAllowedDomains...),
				StrictMode:     true,
				ScanContent:    true,
				WrapContent:    true,
				ScanTimeoutSec: 5,
			},
		},
		Profiles: ProfilesConfig{
			BaseDir:        filepath.Join(userConfigDir(), "profiles"),
			DefaultProfile: "default",
		},
		MultiInstance: MultiInstanceConfig{
			Strategy:          "always-on",
			AllocationPolicy:  "fcfs",
			InstancePortStart: &start,
			InstancePortEnd:   &end,
			Restart: MultiInstanceRestartConfig{
				MaxRestarts:    &restartMaxRestarts,
				InitBackoffSec: &restartInitBackoffSec,
				MaxBackoffSec:  &restartMaxBackoffSec,
				StableAfterSec: &restartStableAfterSec,
			},
		},
		Timeouts: TimeoutsConfig{
			ActionSec:   30,
			NavigateSec: 60,
			ShutdownSec: 10,
			WaitNavMs:   1000,
		},
		Observability: ObservabilityFileConfig{
			Activity: ActivityFileConfig{
				Enabled:        &activityEnabled,
				SessionIdleSec: &activitySessionIdleSec,
				RetentionDays:  &activityRetentionDays,
				StateDir:       "",
			},
		},
		Sessions: SessionsFileConfig{
			Dashboard: DashboardSessionFileConfig{
				Persist:                       &dashboardSessionPersist,
				IdleTimeoutSec:                &dashboardSessionIdleSec,
				MaxLifetimeSec:                &dashboardSessionMaxLifetimeSec,
				ElevationWindowSec:            &dashboardSessionElevationWindowSec,
				PersistElevationAcrossRestart: &dashboardSessionPersistElevationAcrossRestart,
				RequireElevation:              &dashboardSessionRequireElevation,
			},
		},
	}
}

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
	Mode              string `json:"mode"`
	NoRestore         *bool  `json:"noRestore"`
	Timezone          string `json:"timezone"`
	BlockImages       *bool  `json:"blockImages"`
	BlockMedia        *bool  `json:"blockMedia"`
	BlockAds          *bool  `json:"blockAds"`
	MaxTabs           *int   `json:"maxTabs"`
	MaxParallelTabs   *int   `json:"maxParallelTabs"`
	UserAgent         string `json:"userAgent"`
	NoAnimations      *bool  `json:"noAnimations"`
	StealthLevel      string `json:"stealthLevel"`
	TabEvictionPolicy string `json:"tabEvictionPolicy"`
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
	DownloadAllowedDomains []string       `json:"downloadAllowedDomains"`
	DownloadMaxBytes       *int           `json:"downloadMaxBytes"`
	AllowUpload            *bool          `json:"allowUpload"`
	AllowClipboard         *bool          `json:"allowClipboard"`
	UploadMaxRequestBytes  *int           `json:"uploadMaxRequestBytes"`
	UploadMaxFiles         *int           `json:"uploadMaxFiles"`
	UploadMaxFileBytes     *int           `json:"uploadMaxFileBytes"`
	UploadMaxTotalBytes    *int           `json:"uploadMaxTotalBytes"`
	MaxRedirects           *int           `json:"maxRedirects"`
	TrustedProxyCIDRs      []string       `json:"trustedProxyCIDRs"`
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
	AllowedDomains  []string `json:"allowedDomains"`
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
	Enabled        *bool  `json:"enabled"`
	SessionIdleSec *int   `json:"sessionIdleSec"`
	RetentionDays  *int   `json:"retentionDays"`
	StateDir       string `json:"stateDir"`
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
	Enabled     *bool                   `json:"enabled,omitempty"`
	MaxAttempts *int                    `json:"maxAttempts,omitempty"`
	Solvers     []string                `json:"solvers,omitempty"`
	LLMProvider string                  `json:"llmProvider,omitempty"`
	LLMFallback *bool                   `json:"llmFallback,omitempty"`
	External    autoSolverExtConfigJSON `json:"external,omitempty"`
}

type autoSolverExtConfigJSON struct {
	CapsolverKey  string `json:"capsolverKey,omitempty"`
	TwoCaptchaKey string `json:"twoCaptchaKey,omitempty"`
}

func copyStringSlice(items []string) []string {
	if items == nil {
		return []string{}
	}
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items...)
}

func (fc FileConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(fileConfigJSON{
		ConfigVersion: fc.ConfigVersion,
		Server: serverConfigJSON{
			Port:              fc.Server.Port,
			Bind:              fc.Server.Bind,
			Token:             fc.Server.Token,
			StateDir:          fc.Server.StateDir,
			Engine:            fc.Server.Engine,
			NetworkBufferSize: fc.Server.NetworkBufferSize,
			TrustProxyHeaders: fc.Server.TrustProxyHeaders,
			CookieSecure:      fc.Server.CookieSecure,
		},
		Browser: browserConfigJSON{
			ChromeVersion:    fc.Browser.ChromeVersion,
			ChromeBinary:     fc.Browser.ChromeBinary,
			ChromeDebugPort:  fc.Browser.ChromeDebugPort,
			ChromeExtraFlags: fc.Browser.ChromeExtraFlags,
			ExtensionPaths:   copyStringSlice(fc.Browser.ExtensionPaths),
		},
		InstanceDefaults: instanceDefaultsConfigJSON{
			Mode:              fc.InstanceDefaults.Mode,
			NoRestore:         fc.InstanceDefaults.NoRestore,
			Timezone:          fc.InstanceDefaults.Timezone,
			BlockImages:       fc.InstanceDefaults.BlockImages,
			BlockMedia:        fc.InstanceDefaults.BlockMedia,
			BlockAds:          fc.InstanceDefaults.BlockAds,
			MaxTabs:           fc.InstanceDefaults.MaxTabs,
			MaxParallelTabs:   fc.InstanceDefaults.MaxParallelTabs,
			UserAgent:         fc.InstanceDefaults.UserAgent,
			NoAnimations:      fc.InstanceDefaults.NoAnimations,
			StealthLevel:      fc.InstanceDefaults.StealthLevel,
			TabEvictionPolicy: fc.InstanceDefaults.TabEvictionPolicy,
		},
		Security: securityConfigJSON{
			AllowEvaluate:          fc.Security.AllowEvaluate,
			AllowMacro:             fc.Security.AllowMacro,
			AllowScreencast:        fc.Security.AllowScreencast,
			AllowDownload:          fc.Security.AllowDownload,
			DownloadAllowedDomains: copyStringSlice(fc.Security.DownloadAllowedDomains),
			DownloadMaxBytes:       fc.Security.DownloadMaxBytes,
			AllowUpload:            fc.Security.AllowUpload,
			AllowClipboard:         fc.Security.AllowClipboard,
			UploadMaxRequestBytes:  fc.Security.UploadMaxRequestBytes,
			UploadMaxFiles:         fc.Security.UploadMaxFiles,
			UploadMaxFileBytes:     fc.Security.UploadMaxFileBytes,
			UploadMaxTotalBytes:    fc.Security.UploadMaxTotalBytes,
			MaxRedirects:           fc.Security.MaxRedirects,
			TrustedProxyCIDRs:      copyStringSlice(fc.Security.TrustedProxyCIDRs),
			Attach: attachJSON{
				Enabled:      fc.Security.Attach.Enabled,
				AllowHosts:   copyStringSlice(fc.Security.Attach.AllowHosts),
				AllowSchemes: copyStringSlice(fc.Security.Attach.AllowSchemes),
			},
			IDPI: idpiConfigJSON{
				Enabled:         fc.Security.IDPI.Enabled,
				AllowedDomains:  copyStringSlice(fc.Security.IDPI.AllowedDomains),
				StrictMode:      fc.Security.IDPI.StrictMode,
				ScanContent:     fc.Security.IDPI.ScanContent,
				WrapContent:     fc.Security.IDPI.WrapContent,
				CustomPatterns:  copyStringSlice(fc.Security.IDPI.CustomPatterns),
				ScanTimeoutSec:  fc.Security.IDPI.ScanTimeoutSec,
				ShieldThreshold: fc.Security.IDPI.ShieldThreshold,
			},
		},
		Profiles: profilesConfigJSON{
			BaseDir:        fc.Profiles.BaseDir,
			DefaultProfile: fc.Profiles.DefaultProfile,
		},
		MultiInstance: multiInstanceConfigJSON{
			Strategy:          fc.MultiInstance.Strategy,
			AllocationPolicy:  fc.MultiInstance.AllocationPolicy,
			InstancePortStart: fc.MultiInstance.InstancePortStart,
			InstancePortEnd:   fc.MultiInstance.InstancePortEnd,
			Restart: multiInstanceRestartJSON{
				MaxRestarts:    fc.MultiInstance.Restart.MaxRestarts,
				InitBackoffSec: fc.MultiInstance.Restart.InitBackoffSec,
				MaxBackoffSec:  fc.MultiInstance.Restart.MaxBackoffSec,
				StableAfterSec: fc.MultiInstance.Restart.StableAfterSec,
			},
		},
		Timeouts: timeoutsConfigJSON{
			ActionSec:   fc.Timeouts.ActionSec,
			NavigateSec: fc.Timeouts.NavigateSec,
			ShutdownSec: fc.Timeouts.ShutdownSec,
			WaitNavMs:   fc.Timeouts.WaitNavMs,
		},
		Scheduler: schedulerFileConfigJSON{
			Enabled:           fc.Scheduler.Enabled,
			Strategy:          fc.Scheduler.Strategy,
			MaxQueueSize:      fc.Scheduler.MaxQueueSize,
			MaxPerAgent:       fc.Scheduler.MaxPerAgent,
			MaxInflight:       fc.Scheduler.MaxInflight,
			MaxPerAgentFlight: fc.Scheduler.MaxPerAgentFlight,
			ResultTTLSec:      fc.Scheduler.ResultTTLSec,
			WorkerCount:       fc.Scheduler.WorkerCount,
		},
		Observability: observabilityFileConfigJSON{
			Activity: activityConfigJSON{
				Enabled:        fc.Observability.Activity.Enabled,
				SessionIdleSec: fc.Observability.Activity.SessionIdleSec,
				RetentionDays:  fc.Observability.Activity.RetentionDays,
			},
		},
		Sessions: sessionsFileConfigJSON{
			Dashboard: dashboardSessionConfigJSON{
				Persist:                       fc.Sessions.Dashboard.Persist,
				IdleTimeoutSec:                fc.Sessions.Dashboard.IdleTimeoutSec,
				MaxLifetimeSec:                fc.Sessions.Dashboard.MaxLifetimeSec,
				ElevationWindowSec:            fc.Sessions.Dashboard.ElevationWindowSec,
				PersistElevationAcrossRestart: fc.Sessions.Dashboard.PersistElevationAcrossRestart,
				RequireElevation:              fc.Sessions.Dashboard.RequireElevation,
			},
		},
		AutoSolver: autoSolverFileConfigJSON{
			Enabled:     fc.AutoSolver.Enabled,
			MaxAttempts: fc.AutoSolver.MaxAttempts,
			Solvers:     copyStringSlice(fc.AutoSolver.Solvers),
			LLMProvider: fc.AutoSolver.LLMProvider,
			LLMFallback: fc.AutoSolver.LLMFallback,
			External: autoSolverExtConfigJSON{
				CapsolverKey:  fc.AutoSolver.External.CapsolverKey,
				TwoCaptchaKey: fc.AutoSolver.External.TwoCaptchaKey,
			},
		},
	})
}

// FileConfigFromRuntime converts the effective runtime configuration back into a
// nested file configuration shape.
func FileConfigFromRuntime(cfg *RuntimeConfig) FileConfig {
	if cfg == nil {
		return DefaultFileConfig()
	}

	noRestore := cfg.NoRestore
	blockImages := cfg.BlockImages
	blockMedia := cfg.BlockMedia
	blockAds := cfg.BlockAds
	maxTabs := cfg.MaxTabs
	maxParallelTabs := cfg.MaxParallelTabs
	noAnimations := cfg.NoAnimations
	allowEvaluate := cfg.AllowEvaluate
	allowMacro := cfg.AllowMacro
	allowScreencast := cfg.AllowScreencast
	allowDownload := cfg.AllowDownload
	downloadAllowedDomains := copyStringSlice(cfg.DownloadAllowedDomains)
	downloadMaxBytes := cfg.EffectiveDownloadMaxBytes()
	allowUpload := cfg.AllowUpload
	allowClipboard := cfg.AllowClipboard
	uploadMaxRequestBytes := cfg.EffectiveUploadMaxRequestBytes()
	uploadMaxFiles := cfg.EffectiveUploadMaxFiles()
	uploadMaxFileBytes := cfg.EffectiveUploadMaxFileBytes()
	uploadMaxTotalBytes := cfg.EffectiveUploadMaxTotalBytes()
	maxRedirects := cfg.MaxRedirects
	attachEnabled := cfg.AttachEnabled
	start := cfg.InstancePortStart
	end := cfg.InstancePortEnd
	restartMaxRestarts := cfg.RestartMaxRestarts
	restartInitBackoffSec := int(cfg.RestartInitBackoff / time.Second)
	restartMaxBackoffSec := int(cfg.RestartMaxBackoff / time.Second)
	restartStableAfterSec := int(cfg.RestartStableAfter / time.Second)
	activityEnabled := cfg.Observability.Activity.Enabled
	activitySessionIdleSec := cfg.Observability.Activity.SessionIdleSec
	activityRetentionDays := cfg.Observability.Activity.RetentionDays
	dashboardSessionPersist := cfg.Sessions.Dashboard.Persist
	dashboardSessionIdleSec := int(cfg.Sessions.Dashboard.IdleTimeout / time.Second)
	dashboardSessionMaxLifetimeSec := int(cfg.Sessions.Dashboard.MaxLifetime / time.Second)
	dashboardSessionElevationWindowSec := int(cfg.Sessions.Dashboard.ElevationWindow / time.Second)
	dashboardSessionPersistElevationAcrossRestart := cfg.Sessions.Dashboard.PersistElevationAcrossRestart
	dashboardSessionRequireElevation := cfg.Sessions.Dashboard.RequireElevation

	mode := "headless"
	if !cfg.Headless {
		mode = "headed"
	}

	var netBufSize *int
	if cfg.NetworkBufferSize > 0 {
		v := cfg.NetworkBufferSize
		netBufSize = &v
	}

	fc := FileConfig{
		Server: ServerConfig{
			Port:              cfg.Port,
			Bind:              cfg.Bind,
			Token:             cfg.Token,
			StateDir:          cfg.StateDir,
			Engine:            cfg.Engine,
			NetworkBufferSize: netBufSize,
			TrustProxyHeaders: &cfg.TrustProxyHeaders,
			CookieSecure:      cfg.CookieSecure,
		},
		Browser: BrowserConfig{
			ChromeVersion:    cfg.ChromeVersion,
			ChromeBinary:     cfg.ChromeBinary,
			ChromeDebugPort:  intPtrIfPositive(cfg.ChromeDebugPort),
			ChromeExtraFlags: cfg.ChromeExtraFlags,
			ExtensionPaths:   append([]string(nil), cfg.ExtensionPaths...),
		},
		InstanceDefaults: InstanceDefaultsConfig{
			Mode:              mode,
			NoRestore:         &noRestore,
			Timezone:          cfg.Timezone,
			BlockImages:       &blockImages,
			BlockMedia:        &blockMedia,
			BlockAds:          &blockAds,
			MaxTabs:           &maxTabs,
			MaxParallelTabs:   &maxParallelTabs,
			UserAgent:         cfg.UserAgent,
			NoAnimations:      &noAnimations,
			StealthLevel:      cfg.StealthLevel,
			TabEvictionPolicy: cfg.TabEvictionPolicy,
		},
		Security: SecurityConfig{
			AllowEvaluate:          &allowEvaluate,
			AllowMacro:             &allowMacro,
			AllowScreencast:        &allowScreencast,
			AllowDownload:          &allowDownload,
			DownloadAllowedDomains: downloadAllowedDomains,
			DownloadMaxBytes:       &downloadMaxBytes,
			AllowUpload:            &allowUpload,
			AllowClipboard:         &allowClipboard,
			UploadMaxRequestBytes:  &uploadMaxRequestBytes,
			UploadMaxFiles:         &uploadMaxFiles,
			UploadMaxFileBytes:     &uploadMaxFileBytes,
			UploadMaxTotalBytes:    &uploadMaxTotalBytes,
			MaxRedirects:           &maxRedirects,
			TrustedProxyCIDRs:      append([]string(nil), cfg.TrustedProxyCIDRs...),
			Attach: AttachConfig{
				Enabled:      &attachEnabled,
				AllowHosts:   append([]string(nil), cfg.AttachAllowHosts...),
				AllowSchemes: append([]string(nil), cfg.AttachAllowSchemes...),
			},
			IDPI: cfg.IDPI,
		},
		Profiles: ProfilesConfig{
			BaseDir:        cfg.ProfilesBaseDir,
			DefaultProfile: cfg.DefaultProfile,
		},
		MultiInstance: MultiInstanceConfig{
			Strategy:          cfg.Strategy,
			AllocationPolicy:  cfg.AllocationPolicy,
			InstancePortStart: &start,
			InstancePortEnd:   &end,
			Restart: MultiInstanceRestartConfig{
				MaxRestarts:    &restartMaxRestarts,
				InitBackoffSec: &restartInitBackoffSec,
				MaxBackoffSec:  &restartMaxBackoffSec,
				StableAfterSec: &restartStableAfterSec,
			},
		},
		Timeouts: TimeoutsConfig{
			ActionSec:   int(cfg.ActionTimeout / time.Second),
			NavigateSec: int(cfg.NavigateTimeout / time.Second),
			ShutdownSec: int(cfg.ShutdownTimeout / time.Second),
			WaitNavMs:   int(cfg.WaitNavDelay / time.Millisecond),
		},
		Observability: ObservabilityFileConfig{
			Activity: ActivityFileConfig{
				Enabled:        &activityEnabled,
				SessionIdleSec: &activitySessionIdleSec,
				RetentionDays:  &activityRetentionDays,
			},
		},
		Sessions: SessionsFileConfig{
			Dashboard: DashboardSessionFileConfig{
				Persist:                       &dashboardSessionPersist,
				IdleTimeoutSec:                &dashboardSessionIdleSec,
				MaxLifetimeSec:                &dashboardSessionMaxLifetimeSec,
				ElevationWindowSec:            &dashboardSessionElevationWindowSec,
				PersistElevationAcrossRestart: &dashboardSessionPersistElevationAcrossRestart,
				RequireElevation:              &dashboardSessionRequireElevation,
			},
		},
	}

	return fc
}

func intPtrIfPositive(v int) *int {
	if v <= 0 {
		return nil
	}
	n := v
	return &n
}

// legacyFileConfig is the old flat structure for backward compatibility.
type legacyFileConfig struct {
	Port              string `json:"port"`
	InstancePortStart *int   `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int   `json:"instancePortEnd,omitempty"`
	Token             string `json:"token,omitempty"`
	AllowEvaluate     *bool  `json:"allowEvaluate,omitempty"`
	AllowMacro        *bool  `json:"allowMacro,omitempty"`
	AllowScreencast   *bool  `json:"allowScreencast,omitempty"`
	AllowDownload     *bool  `json:"allowDownload,omitempty"`
	AllowUpload       *bool  `json:"allowUpload,omitempty"`
	AllowClipboard    *bool  `json:"allowClipboard,omitempty"`
	StateDir          string `json:"stateDir"`
	ProfileDir        string `json:"profileDir"`
	Headless          *bool  `json:"headless,omitempty"`
	NoRestore         bool   `json:"noRestore"`
	MaxTabs           *int   `json:"maxTabs,omitempty"`
	TimeoutSec        int    `json:"timeoutSec,omitempty"`
	NavigateSec       int    `json:"navigateSec,omitempty"`
}

// convertLegacyConfig converts flat config to nested structure.
func convertLegacyConfig(lc *legacyFileConfig) *FileConfig {
	fc := &FileConfig{}

	// Server
	fc.Server.Port = lc.Port
	fc.Server.Token = lc.Token
	fc.Server.StateDir = lc.StateDir

	// Browser / instance defaults
	if lc.Headless != nil {
		if *lc.Headless {
			fc.InstanceDefaults.Mode = "headless"
		} else {
			fc.InstanceDefaults.Mode = "headed"
		}
	}
	fc.InstanceDefaults.MaxTabs = lc.MaxTabs
	if lc.NoRestore {
		b := true
		fc.InstanceDefaults.NoRestore = &b
	}

	// Profiles
	if lc.ProfileDir != "" {
		fc.Profiles.BaseDir = filepath.Dir(lc.ProfileDir)
		fc.Profiles.DefaultProfile = filepath.Base(lc.ProfileDir)
	}

	// Security
	fc.Security.AllowEvaluate = lc.AllowEvaluate
	fc.Security.AllowMacro = lc.AllowMacro
	fc.Security.AllowScreencast = lc.AllowScreencast
	fc.Security.AllowDownload = lc.AllowDownload
	fc.Security.AllowUpload = lc.AllowUpload
	fc.Security.AllowClipboard = lc.AllowClipboard

	// Timeouts
	fc.Timeouts.ActionSec = lc.TimeoutSec
	fc.Timeouts.NavigateSec = lc.NavigateSec

	// Multi-instance
	fc.MultiInstance.InstancePortStart = lc.InstancePortStart
	fc.MultiInstance.InstancePortEnd = lc.InstancePortEnd

	return fc
}

// isLegacyConfig detects if JSON is flat (legacy) or nested (new).
func isLegacyConfig(data []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}

	// If any new nested keys exist, it's new format
	newKeys := []string{"server", "browser", "instanceDefaults", "profiles", "multiInstance", "security", "attach", "timeouts", "sessions"}
	for _, key := range newKeys {
		if _, has := probe[key]; has {
			return false
		}
	}

	// If "port" or "headless" exist at top level, it's legacy
	if _, hasPort := probe["port"]; hasPort {
		return true
	}
	if _, hasHeadless := probe["headless"]; hasHeadless {
		return true
	}

	return false
}

func modeToHeadless(mode string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return fallback
	case "headless":
		return true
	case "headed":
		return false
	default:
		return fallback
	}
}
