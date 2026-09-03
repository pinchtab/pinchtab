package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/browsers"
)

var configHintOnce sync.Once

// EmitDefaultConfigHint prints a one-time hint to stderr when PINCHTAB_CONFIG
// points somewhere other than the default config path AND a default config
// already exists at that path. The hint is best-effort UX nudge for users
// who may not realize they're running against a custom config.
//
// Scoped to specific commands (health, config) and once-per-process via
// sync.Once so scripted callers and unrelated CLI commands stay quiet.
func EmitDefaultConfigHint() {
	configHintOnce.Do(func() {
		defaultConfigPath := filepath.Join(userConfigDir(), "config.json")
		configPath := envOr("PINCHTAB_CONFIG", defaultConfigPath)
		if configPath == defaultConfigPath {
			return
		}
		if _, err := os.Stat(defaultConfigPath); err != nil {
			return
		}
		fmt.Fprintf(os.Stderr, "HINT: default config exists at %s — you can edit it directly instead of using PINCHTAB_CONFIG\n", defaultConfigPath)
	})
}

// parsedConfigFile is the side-effect-free result of resolving + reading +
// parsing the config file.
type parsedConfigFile struct {
	Path           string
	DefaultPath    string
	EnvOverride    bool
	Found          bool  // file read succeeded
	ReadErr        error // os.ReadFile error (incl. not-exist); nil when Found
	Legacy         bool
	FC             *FileConfig
	ParseErr       error    // json unmarshal error (legacy or nested)
	UnknownFields  error    // non-fatal: unrecognized nested fields
	UnknownKeys    []string // the dotted paths behind UnknownFields
	ValidationErrs []error  // non-fatal: ValidateFileConfig
	Advisories     []string // non-gating: FileConfigAdvisories
}

func resolveConfigPath() (path, defaultPath string, envOverride bool) {
	defaultPath = filepath.Join(userConfigDir(), "config.json")
	path = envOr("PINCHTAB_CONFIG", defaultPath)
	return path, defaultPath, os.Getenv("PINCHTAB_CONFIG") != ""
}

// ConfigFilePath returns the effective config file path (honoring PINCHTAB_CONFIG),
// i.e. the path LoadFileConfig reads — exposed so callers can stat it for change
// detection without performing a full load.
func ConfigFilePath() string {
	path, _, _ := resolveConfigPath()
	return path
}

// readAndParseConfigFile resolves, reads, detects legacy format, and parses the
// config file with no side effects (no logging, no os.Exit). Callers decide how
// to surface the outcome.
func readAndParseConfigFile() parsedConfigFile {
	path, defaultPath, override := resolveConfigPath()
	res := parsedConfigFile{Path: path, DefaultPath: defaultPath, EnvOverride: override}

	data, err := os.ReadFile(path)
	if err != nil {
		res.ReadErr = err
		return res
	}
	res.Found = true

	if isLegacyConfig(data) {
		res.Legacy = true
		var lc legacyFileConfig
		if err := json.Unmarshal(data, &lc); err != nil {
			res.ParseErr = err
			return res
		}
		res.FC = convertLegacyConfig(&lc)
	} else {
		fc := &FileConfig{}
		if err := json.Unmarshal(data, fc); err != nil {
			res.ParseErr = err
			return res
		}
		res.FC = fc
		if keys := UnknownFileConfigKeys(data); len(keys) > 0 {
			res.UnknownKeys = keys
			res.UnknownFields = &UnknownConfigKeysError{Keys: keys}
		}
	}
	if res.FC != nil {
		res.ValidationErrs = ValidateFileConfig(res.FC)
		res.Advisories = FileConfigAdvisories(res.FC)
	}
	return res
}

// LoadDiagnostic is a non-fatal config-load message for the caller to emit.
type LoadDiagnostic struct {
	Level   slog.Level
	Message string
	Attrs   []any // slog key/value pairs
}

// Load returns the RuntimeConfig with precedence: env vars > config file >
// defaults. It is a thin wrapper over LoadConfig that emits the load diagnostics
// via slog and terminates the process on a fatal config error.
func Load() *RuntimeConfig {
	cfg, diags := LoadDeferringDiagnostics()
	EmitLoadDiagnostics(diags)
	return cfg
}

// LoadDeferringDiagnostics loads the config and hands back its diagnostics
// unemitted, for a caller that must resolve the log level before they are
// written. The load diagnostics describe reading the very file that carries
// server.logLevel, so emitting them during the load drops every debug one at any
// setting. A fatal config error still terminates here: it is reported at error
// level, which no level suppresses, and there is nothing to resolve afterwards.
//
// This carries no precedence logic — the caller decides the level and then calls
// EmitLoadDiagnostics.
func LoadDeferringDiagnostics() (*RuntimeConfig, []LoadDiagnostic) {
	cfg, diags, err := LoadConfig()
	if err != nil {
		EmitLoadDiagnostics(diags)
		slog.Error(err.Error())
		os.Exit(1)
	}
	return cfg, diags
}

// EmitLoadDiagnostics writes collected load diagnostics through slog at the level
// each one declares.
func EmitLoadDiagnostics(diags []LoadDiagnostic) {
	for _, d := range diags {
		switch d.Level {
		case slog.LevelDebug:
			slog.Debug(d.Message, d.Attrs...)
		case slog.LevelInfo:
			slog.Info(d.Message, d.Attrs...)
		default:
			slog.Warn(d.Message, d.Attrs...)
		}
	}
}

// LoadConfig builds the RuntimeConfig (env > file > defaults) with no logging and
// no os.Exit. It returns the config, ordered diagnostics for the caller to log,
// and a fatal error (e.g. missing port) for the caller to act on.
// defaultAutoSolverConfig is the single owner of the autoSolver section's defaults.
// LoadConfig assigns it directly and DefaultFileConfig pointer-wraps its fields —
// the plain/pointer split stays, because pointer-wrapping is how "absent from file"
// remains distinguishable from "explicitly false", but the VALUES are written once.
// Everything the core also owns is read from autosolver.DefaultConfig() with the unit
// conversion in this one place; only the three trigger flags, which the core has no
// counterpart for, are literals here.
func defaultAutoSolverConfig() AutoSolverConfig {
	core := autosolver.DefaultConfig()
	return AutoSolverConfig{
		Enabled:           core.Enabled,
		AutoTrigger:       true,
		TriggerOnNavigate: true,
		TriggerOnAction:   true,
		MaxAttempts:       core.MaxAttempts,
		SolverTimeoutSec:  int(core.SolverTimeout / time.Second),
		RetryBaseDelayMs:  int(core.RetryBaseDelay / time.Millisecond),
		RetryMaxDelayMs:   int(core.RetryMaxDelay / time.Millisecond),
		Solvers:           core.Solvers,
		LLMFallback:       core.LLMFallback,
	}
}

func LoadConfig() (*RuntimeConfig, []LoadDiagnostic, error) {
	cfg := &RuntimeConfig{
		Bind:              "127.0.0.1",
		Port:              defaultPort,
		InstancePortStart: 9868,
		InstancePortEnd:   9968,
		Token:             os.Getenv("PINCHTAB_TOKEN"),
		StateDir:          userConfigDir(),
		CookieSecure:      nil,

		AllowEvaluate:             false,
		AllowMacro:                false,
		AllowScreencast:           false,
		AllowDownload:             false,
		AllowCookies:              false,
		AllowNetworkIntercept:     false,
		AllowFileScheme:           false,
		RetainNetworkBodies:       false,
		RetainNetworkBodyMaxBytes: 256 * 1024,
		AllowedDomains:            append([]string(nil), defaultLocalAllowedDomains...),
		DownloadAllowedDomains:    nil,
		DownloadMaxBytes:          DefaultDownloadMaxBytes,
		AllowUpload:               false,
		AllowClipboard:            false,
		AllowStateExport:          false,
		StateEncryptionKey:        "",
		EnableActionGuards:        true,
		UploadMaxRequestBytes:     DefaultUploadMaxRequestBytes,
		UploadMaxFiles:            DefaultUploadMaxFiles,
		UploadMaxFileBytes:        DefaultUploadMaxFileBytes,
		UploadMaxTotalBytes:       DefaultUploadMaxTotalBytes,
		MaxRedirects:              -1, // Unlimited by default; set to N to limit redirect hops

		Headless:               true,
		NoRestore:              false,
		ProfileDir:             "",
		ProfilesBaseDir:        "",
		DefaultProfile:         "default",
		ProfileQuarantineKeep:  DefaultProfileQuarantineKeep,
		Timezone:               "",
		BlockImages:            false,
		BlockMedia:             false,
		BlockAds:               false,
		MaxTabs:                20,
		MaxParallelTabs:        0,
		DefaultBrowser:         DefaultBrowserForSystem(),
		BrowserBinary:          "", // Set via config.json only
		BrowserExtraFlags:      "",
		Cloak:                  CloakBrowserRuntimeConfig{DisableDefaultStealthArgs: true},
		ExtensionPaths:         []string{defaultExtensionsDir(userConfigDir())},
		UserAgent:              "",
		NoAnimations:           false,
		CaptureAllowActivation: true,
		Humanize:               false,
		StealthLevel:           "light",
		TabEvictionPolicy:      "close_lru",
		TabLifecyclePolicy:     "keep",
		TabCloseDelay:          5 * time.Minute,
		TabRestore:             false,

		ActionTimeout:   30 * time.Second,
		NavigateTimeout: 60 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		WaitNavDelay:    1 * time.Second,

		Strategy:           "always-on",
		AllocationPolicy:   "fcfs",
		RestartMaxRestarts: 20,
		RestartInitBackoff: 2 * time.Second,
		RestartMaxBackoff:  60 * time.Second,
		RestartStableAfter: 5 * time.Minute,

		AttachEnabled:      false,
		AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
		AttachAllowSchemes: []string{"ws", "wss", "http", "https"},

		IDPI: IDPIConfig{
			Enabled:        true,
			StrictMode:     true,
			ScanContent:    true,
			WrapContent:    true,
			ScanTimeoutSec: 5,
		},

		Observability: ObservabilityConfig{
			Activity: ActivityConfig{
				Enabled:        true,
				SessionIdleSec: 1800,
				RetentionDays:  30,
				StateDir:       "",
			},
		},

		Sessions: SessionsRuntimeConfig{
			Agent: AgentSessionRuntimeConfig{
				Enabled:     DefaultAgentSessionsEnabled,
				Mode:        "preferred",
				IdleTimeout: 30 * time.Minute,
				MaxLifetime: 24 * time.Hour,
			},
			Dashboard: DashboardSessionRuntimeConfig{
				Persist:                       true,
				IdleTimeout:                   7 * 24 * time.Hour,
				MaxLifetime:                   7 * 24 * time.Hour,
				ElevationWindow:               15 * time.Minute,
				PersistElevationAcrossRestart: false,
				RequireElevation:              false,
			},
		},

		AutoSolver: defaultAutoSolverConfig(),
	}

	// Deferred, not called here, because the profile paths are DERIVED from
	// server.stateDir and the file that carries stateDir has not been read yet.
	// Finalizing eagerly filled ProfilesBaseDir from the default state dir, and the
	// second call after applyFileConfig then found it non-empty and left it alone — so
	// server.stateDir could never relocate profiles. A defer is the one shape that
	// cannot miss one of this function's several early returns.
	defer finalizeProfileConfig(cfg)
	defer finalizeSchedulerConfig(cfg)

	var diags []LoadDiagnostic
	res := readAndParseConfigFile()
	if !res.Found {
		if res.ReadErr != nil && !os.IsNotExist(res.ReadErr) {
			diags = append(diags, LoadDiagnostic{slog.LevelWarn, "failed to read config file", []any{"path", res.Path, "error", res.ReadErr}})
		}
		return cfg, diags, nil
	}

	diags = append(diags, LoadDiagnostic{slog.LevelDebug, "loading config file", []any{"path", res.Path}})

	if res.ParseErr != nil {
		if res.Legacy {
			diags = append(diags, LoadDiagnostic{slog.LevelWarn, "failed to parse legacy config", []any{"path", res.Path, "error", res.ParseErr}})
		} else {
			diags = append(diags, LoadDiagnostic{slog.LevelWarn, "failed to parse config", []any{"path", res.Path, "error", res.ParseErr}})
		}
		return cfg, diags, nil
	}
	if res.Legacy {
		diags = append(diags, LoadDiagnostic{slog.LevelInfo, "loaded legacy flat config, consider migrating to nested format", []any{"path", res.Path}})
	}
	if res.UnknownFields != nil {
		diags = append(diags, LoadDiagnostic{slog.LevelWarn, "config has unrecognized fields that will be ignored", []any{"path", res.Path, "error", res.UnknownFields}})
	}
	for _, e := range res.ValidationErrs {
		diags = append(diags, LoadDiagnostic{slog.LevelWarn, "config validation error", []any{"path", res.Path, "error", e}})
	}
	// Reported at load like the rest, but from the non-gating list: the file says
	// something inert, which is worth knowing and is nobody's blocker.
	for _, advisory := range res.Advisories {
		diags = append(diags, LoadDiagnostic{slog.LevelWarn, "config setting has no effect", []any{"path", res.Path, "advisory", advisory}})
	}

	diags = append(diags, applyFileConfig(cfg, res.FC)...)

	if cfg.Port == "" {
		return cfg, diags, fmt.Errorf("server port is not configured — set server.port in config.json")
	}

	return cfg, diags, nil
}

// ConfigFileStatus reports on-disk config state without invoking Load (used by `pinchtab doctor`).
type ConfigFileStatus struct {
	Path        string
	DefaultPath string
	EnvOverride bool
	Found       bool
	ParseErr    error
	UnknownKeys []string
}

// InspectConfigFile reports config-file load status with no side effects.
func InspectConfigFile() ConfigFileStatus {
	res := readAndParseConfigFile()
	return ConfigFileStatus{
		Path:        res.Path,
		DefaultPath: res.DefaultPath,
		EnvOverride: res.EnvOverride,
		Found:       res.Found,
		ParseErr:    res.ParseErr,
		UnknownKeys: res.UnknownKeys,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func finalizeProfileConfig(cfg *RuntimeConfig) {
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "default"
	}
	if cfg.ProfilesBaseDir == "" {
		cfg.ProfilesBaseDir = filepath.Join(cfg.StateDir, "profiles")
	}
	if cfg.ProfileDir == "" {
		cfg.ProfileDir = filepath.Join(cfg.ProfilesBaseDir, cfg.DefaultProfile)
	}
}

func applyFileConfig(cfg *RuntimeConfig, fc *FileConfig) []LoadDiagnostic {
	applyServerConfig(cfg, fc.Server)
	applySecurityConfig(cfg, fc.Security)
	applyObservabilityConfig(cfg, fc.Observability)
	applySessionsConfig(cfg, fc.Sessions)
	diags := applyBrowserConfig(cfg, &fc.Browser, fc.Browsers)
	applyInstanceDefaultsConfig(cfg, &fc.InstanceDefaults)
	applyProfilesConfig(cfg, fc.Profiles)
	applyMultiInstanceConfig(cfg, fc.MultiInstance)
	applyTimeoutsConfig(cfg, fc.Timeouts)
	applySchedulerConfig(cfg, fc.Scheduler)
	applyAutoSolverConfig(cfg, fc.AutoSolver)
	return diags
}

func applyServerConfig(cfg *RuntimeConfig, s ServerConfig) {
	if s.Port != "" {
		cfg.Port = s.Port
	}
	if s.Bind != "" {
		cfg.Bind = s.Bind
	}
	if os.Getenv("PINCHTAB_TOKEN") == "" {
		cfg.Token = s.Token
	}
	if s.StateDir != "" {
		cfg.StateDir = s.StateDir
	}
	if s.LogLevel != "" {
		cfg.LogLevel = s.LogLevel
	}
	if s.NetworkBufferSize != nil && *s.NetworkBufferSize > 0 {
		cfg.NetworkBufferSize = ClampNetworkBufferSize(*s.NetworkBufferSize)
	}
	if s.RetainNetworkBodies != nil {
		cfg.RetainNetworkBodies = *s.RetainNetworkBodies
	}
	if s.RetainNetworkBodyMaxBytes != nil && *s.RetainNetworkBodyMaxBytes >= 0 {
		cfg.RetainNetworkBodyMaxBytes = *s.RetainNetworkBodyMaxBytes
	}
	if s.TrustProxyHeaders != nil {
		cfg.TrustProxyHeaders = *s.TrustProxyHeaders
	}
	cfg.CookieSecure = s.CookieSecure
}

func applySecurityConfig(cfg *RuntimeConfig, s SecurityConfig) {
	if s.AllowEvaluate != nil {
		cfg.AllowEvaluate = *s.AllowEvaluate
	}
	if s.AllowMacro != nil {
		cfg.AllowMacro = *s.AllowMacro
	}
	if s.AllowScreencast != nil {
		cfg.AllowScreencast = *s.AllowScreencast
	}
	if s.AllowDownload != nil {
		cfg.AllowDownload = *s.AllowDownload
	}
	if s.AllowCookies != nil {
		cfg.AllowCookies = *s.AllowCookies
	}
	if s.AllowNetworkIntercept != nil {
		cfg.AllowNetworkIntercept = *s.AllowNetworkIntercept
	}
	if s.AllowFileScheme != nil {
		cfg.AllowFileScheme = *s.AllowFileScheme
	}
	cfg.DownloadAllowedDomains = append([]string(nil), s.DownloadAllowedDomains...)
	if s.DownloadMaxBytes != nil {
		cfg.DownloadMaxBytes = clampPositiveLimit(*s.DownloadMaxBytes, DefaultDownloadMaxBytes, MaxDownloadMaxBytes)
	}
	if s.AllowUpload != nil {
		cfg.AllowUpload = *s.AllowUpload
	}
	if s.AllowClipboard != nil {
		cfg.AllowClipboard = *s.AllowClipboard
	}
	if s.AllowStateExport != nil {
		cfg.AllowStateExport = *s.AllowStateExport
	}
	if s.StateEncryptionKey != nil {
		cfg.StateEncryptionKey = *s.StateEncryptionKey
	}
	if s.EnableActionGuards != nil {
		cfg.EnableActionGuards = *s.EnableActionGuards
	}
	if s.UploadMaxRequestBytes != nil {
		cfg.UploadMaxRequestBytes = clampPositiveLimit(*s.UploadMaxRequestBytes, DefaultUploadMaxRequestBytes, MaxUploadMaxRequestBytes)
	}
	if s.UploadMaxFiles != nil {
		cfg.UploadMaxFiles = clampPositiveLimit(*s.UploadMaxFiles, DefaultUploadMaxFiles, MaxUploadMaxFiles)
	}
	if s.UploadMaxFileBytes != nil {
		cfg.UploadMaxFileBytes = clampPositiveLimit(*s.UploadMaxFileBytes, DefaultUploadMaxFileBytes, MaxUploadMaxFileBytes)
	}
	if s.UploadMaxTotalBytes != nil {
		cfg.UploadMaxTotalBytes = clampPositiveLimit(*s.UploadMaxTotalBytes, DefaultUploadMaxTotalBytes, MaxUploadMaxTotalBytes)
	}
	if s.MaxRedirects != nil {
		cfg.MaxRedirects = *s.MaxRedirects
	}
	cfg.TrustedProxyCIDRs = append([]string(nil), s.TrustedProxyCIDRs...)
	cfg.TrustedResolveCIDRs = append([]string(nil), s.TrustedResolveCIDRs...)
	if s.TrustLoopbackProxy != nil {
		cfg.TrustLoopbackProxy = *s.TrustLoopbackProxy
	}
	cfg.IDPI = s.IDPI
	cfg.AllowedDomains = effectiveSecurityAllowedDomains(s)
	if s.Attach.Enabled != nil {
		cfg.AttachEnabled = *s.Attach.Enabled
	}
	if s.Attach.ForwardProxyAuth != nil {
		cfg.AttachForwardProxyAuth = *s.Attach.ForwardProxyAuth
	}
	if len(s.Attach.AllowHosts) > 0 {
		cfg.AttachAllowHosts = append([]string(nil), s.Attach.AllowHosts...)
	}
	if len(s.Attach.AllowSchemes) > 0 {
		cfg.AttachAllowSchemes = append([]string(nil), s.Attach.AllowSchemes...)
	}
}

func applyObservabilityConfig(cfg *RuntimeConfig, o ObservabilityFileConfig) {
	if o.Activity.Enabled != nil {
		cfg.Observability.Activity.Enabled = *o.Activity.Enabled
	}
	if o.Activity.SessionIdleSec != nil {
		cfg.Observability.Activity.SessionIdleSec = *o.Activity.SessionIdleSec
	}
	if o.Activity.RetentionDays != nil {
		cfg.Observability.Activity.RetentionDays = *o.Activity.RetentionDays
	}
	if o.Activity.Events.Dashboard != nil {
		cfg.Observability.Activity.Events.Dashboard = *o.Activity.Events.Dashboard
	}
	if o.Activity.Events.Server != nil {
		cfg.Observability.Activity.Events.Server = *o.Activity.Events.Server
	}
	if o.Activity.Events.Bridge != nil {
		cfg.Observability.Activity.Events.Bridge = *o.Activity.Events.Bridge
	}
	if o.Activity.Events.Orchestrator != nil {
		cfg.Observability.Activity.Events.Orchestrator = *o.Activity.Events.Orchestrator
	}
	if o.Activity.Events.Scheduler != nil {
		cfg.Observability.Activity.Events.Scheduler = *o.Activity.Events.Scheduler
	}
	if o.Activity.Events.MCP != nil {
		cfg.Observability.Activity.Events.MCP = *o.Activity.Events.MCP
	}
	if o.Activity.Events.Other != nil {
		cfg.Observability.Activity.Events.Other = *o.Activity.Events.Other
	}
}

func applySessionsConfig(cfg *RuntimeConfig, s SessionsFileConfig) {
	if s.Dashboard.Persist != nil {
		cfg.Sessions.Dashboard.Persist = *s.Dashboard.Persist
	}
	if s.Dashboard.IdleTimeoutSec != nil && *s.Dashboard.IdleTimeoutSec > 0 {
		cfg.Sessions.Dashboard.IdleTimeout = time.Duration(*s.Dashboard.IdleTimeoutSec) * time.Second
	}
	if s.Dashboard.MaxLifetimeSec != nil && *s.Dashboard.MaxLifetimeSec > 0 {
		cfg.Sessions.Dashboard.MaxLifetime = time.Duration(*s.Dashboard.MaxLifetimeSec) * time.Second
	}
	if s.Dashboard.ElevationWindowSec != nil && *s.Dashboard.ElevationWindowSec > 0 {
		cfg.Sessions.Dashboard.ElevationWindow = time.Duration(*s.Dashboard.ElevationWindowSec) * time.Second
	}
	if s.Dashboard.PersistElevationAcrossRestart != nil {
		cfg.Sessions.Dashboard.PersistElevationAcrossRestart = *s.Dashboard.PersistElevationAcrossRestart
	}
	if s.Dashboard.RequireElevation != nil {
		cfg.Sessions.Dashboard.RequireElevation = *s.Dashboard.RequireElevation
	}
	if s.Agent.Enabled != nil {
		cfg.Sessions.Agent.Enabled = *s.Agent.Enabled
	}
	if s.Agent.Mode != "" {
		cfg.Sessions.Agent.Mode = s.Agent.Mode
	}
	if s.Agent.IdleTimeoutSec != nil && *s.Agent.IdleTimeoutSec > 0 {
		cfg.Sessions.Agent.IdleTimeout = time.Duration(*s.Agent.IdleTimeoutSec) * time.Second
	}
	if s.Agent.MaxLifetimeSec != nil && *s.Agent.MaxLifetimeSec > 0 {
		cfg.Sessions.Agent.MaxLifetime = time.Duration(*s.Agent.MaxLifetimeSec) * time.Second
	}
}

// applyBrowserConfig returns its notices rather than logging them: a debug notice
// emitted during the load is written before any command has resolved the level,
// so logging here made a silently rewritten config unreadable at every setting.
func applyBrowserConfig(cfg *RuntimeConfig, browser *BrowserConfig, browsersCfg BrowsersConfig) []LoadDiagnostic {
	var diags []LoadDiagnostic
	synthesized, conflict := migrateLegacyBrowserConfig(browser, browsersCfg.Default)
	if conflict {
		diags = append(diags, LoadDiagnostic{slog.LevelWarn, "config has both browser.targets and legacy browser.binary/extraFlags/cloak/proxy set; targets are used as authored (no legacy synthesis), but the legacy fields still seed the base runtime config that target resolution overlays per-field", nil})
	} else if synthesized {
		diags = append(diags, LoadDiagnostic{slog.LevelDebug, "migrated legacy browser config into browser.targets.default", nil})
	}
	cfg.Targets = cloneBrowserTargetsConfig(browser.Targets)
	cfg.DefaultTarget = browser.DefaultTarget
	cfg.FallbackOrder = append([]string(nil), browser.FallbackOrder...)
	cfg.TargetsSynthesized = synthesized && len(browser.Targets) > 0

	if browsersCfg.Default != "" {
		cfg.DefaultBrowser = strings.ToLower(strings.TrimSpace(browsersCfg.Default))
		if _, ok := browsers.Get(strings.ToLower(strings.TrimSpace(browsersCfg.Default))); !ok {
			diags = append(diags, LoadDiagnostic{slog.LevelWarn, "browsers.default is not a known browser; launches will fall back to chrome",
				[]any{"configured", browsersCfg.Default, "known", browsers.IDs()}})
		}
	} else if name := ResolveDefaultTarget(cfg); name != "" && !cfg.TargetsSynthesized && cfg.Targets[name].Provider != "" {
		cfg.DefaultBrowser = NormalizeBrowser(cfg.Targets[name].Provider)
	} else {
		cfg.DefaultBrowser = DefaultBrowserForSystem()
	}

	if b, ok := browsers.Get(strings.ToLower(cfg.DefaultBrowser)); ok && b.Capabilities().Has(browsers.CapNativeStealth) && browser.Cloak.DisableDefaultStealthArgs == nil {
		cfg.Cloak.DisableDefaultStealthArgs = true
	}

	if browser.BrowserVersion != "" {
		cfg.BrowserVersion = browser.BrowserVersion
	}
	if browser.BrowserBinary != "" {
		cfg.BrowserBinary = browser.BrowserBinary
	}
	if browser.BrowserDebugPort != nil && *browser.BrowserDebugPort > 0 {
		cfg.BrowserDebugPort = *browser.BrowserDebugPort
	}
	if browser.BrowserExtraFlags != "" {
		cfg.BrowserExtraFlags = SanitizeBrowserExtraFlags(browser.BrowserExtraFlags)
	}
	applyCloakBrowserConfigToRuntime(cfg, browser.Cloak)
	cfg.Proxy = BrowserProxyConfig{
		Server:     browser.Proxy.Server,
		BypassList: append([]string(nil), browser.Proxy.BypassList...),
		Username:   browser.Proxy.Username,
		Password:   browser.Proxy.Password,
	}
	if browser.Proxy.Geo != nil {
		geoCopy := *browser.Proxy.Geo
		cfg.Proxy.Geo = &geoCopy
	}
	if browser.ExtensionPaths != nil {
		cfg.ExtensionPaths = append([]string(nil), browser.ExtensionPaths...)
	}

	if len(browsersCfg.Available) > 0 {
		cfg.BrowsersAvailable = make([]string, len(browsersCfg.Available))
		copy(cfg.BrowsersAvailable, browsersCfg.Available)
	} else if cfg.DefaultBrowser != "" {
		cfg.BrowsersAvailable = []string{cfg.DefaultBrowser}
	} else {
		cfg.BrowsersAvailable = []string{"chrome"}
	}

	return diags
}

func applyInstanceDefaultsConfig(cfg *RuntimeConfig, d *InstanceDefaultsConfig) {
	if d.Headless != nil && d.Mode == "" {
		if *d.Headless {
			d.Mode = "headless"
		} else {
			d.Mode = "headed"
		}
	}
	if d.Mode != "" {
		cfg.Headless = modeToHeadless(d.Mode, cfg.Headless)
		cfg.HeadlessSet = true
	}
	if d.NoRestore != nil {
		cfg.NoRestore = *d.NoRestore
	}
	if d.Timezone != "" {
		cfg.Timezone = d.Timezone
	}
	if d.BlockImages != nil {
		cfg.BlockImages = *d.BlockImages
	}
	if d.BlockMedia != nil {
		cfg.BlockMedia = *d.BlockMedia
	}
	if d.BlockAds != nil {
		cfg.BlockAds = *d.BlockAds
	}
	if d.MaxTabs != nil {
		cfg.MaxTabs = *d.MaxTabs
	}
	if d.MaxParallelTabs != nil {
		cfg.MaxParallelTabs = *d.MaxParallelTabs
	}
	if d.UserAgent != "" {
		cfg.UserAgent = d.UserAgent
	}
	if d.NoAnimations != nil {
		cfg.NoAnimations = *d.NoAnimations
	}
	if d.CaptureAllowActivation != nil {
		cfg.CaptureAllowActivation = *d.CaptureAllowActivation
	}
	if d.Humanize != nil {
		cfg.Humanize = *d.Humanize
	}
	if d.StealthLevel != "" {
		cfg.StealthLevel = d.StealthLevel
	}
	if d.TabEvictionPolicy != "" {
		cfg.TabEvictionPolicy = d.TabEvictionPolicy
	}
	if tp := d.TabPolicy; tp != nil {
		if tp.Eviction != "" {
			cfg.TabEvictionPolicy = tp.Eviction
		}
		if tp.Lifecycle != "" {
			cfg.TabLifecyclePolicy = tp.Lifecycle
		}
		if tp.CloseDelaySec != nil && *tp.CloseDelaySec > 0 {
			cfg.TabCloseDelay = time.Duration(*tp.CloseDelaySec) * time.Second
		}
		if tp.Restore != nil {
			cfg.TabRestore = *tp.Restore
		}
	}
	if cfg.TabLifecyclePolicy == "close_idle" && cfg.TabCloseDelay < time.Second {
		cfg.TabCloseDelay = time.Second
	}
	if d.DialogAutoAccept != nil {
		cfg.DialogAutoAccept = *d.DialogAutoAccept
	}
}

func applyProfilesConfig(cfg *RuntimeConfig, p ProfilesConfig) {
	if p.BaseDir != "" {
		cfg.ProfilesBaseDir = p.BaseDir
	}
	if p.DefaultProfile != "" {
		cfg.DefaultProfile = p.DefaultProfile
	}
	if p.QuarantineKeep != nil && *p.QuarantineKeep >= 0 {
		cfg.ProfileQuarantineKeep = *p.QuarantineKeep
	}
	cfg.ProfileDir = ""
}

func applyMultiInstanceConfig(cfg *RuntimeConfig, m MultiInstanceConfig) {
	if m.Strategy != "" {
		cfg.Strategy = m.Strategy
	}
	if m.AllocationPolicy != "" {
		cfg.AllocationPolicy = m.AllocationPolicy
	}
	if m.InstancePortStart != nil {
		cfg.InstancePortStart = *m.InstancePortStart
	}
	if m.InstancePortEnd != nil {
		cfg.InstancePortEnd = *m.InstancePortEnd
	}
	if m.Restart.MaxRestarts != nil {
		cfg.RestartMaxRestarts = *m.Restart.MaxRestarts
	}
	if m.Restart.InitBackoffSec != nil {
		cfg.RestartInitBackoff = time.Duration(*m.Restart.InitBackoffSec) * time.Second
	}
	if m.Restart.MaxBackoffSec != nil {
		cfg.RestartMaxBackoff = time.Duration(*m.Restart.MaxBackoffSec) * time.Second
	}
	if m.Restart.StableAfterSec != nil {
		cfg.RestartStableAfter = time.Duration(*m.Restart.StableAfterSec) * time.Second
	}
}

func applyTimeoutsConfig(cfg *RuntimeConfig, t TimeoutsConfig) {
	if t.ActionSec > 0 {
		cfg.ActionTimeout = time.Duration(t.ActionSec) * time.Second
	}
	if t.NavigateSec > 0 {
		cfg.NavigateTimeout = time.Duration(t.NavigateSec) * time.Second
	}
	if t.ShutdownSec > 0 {
		cfg.ShutdownTimeout = time.Duration(t.ShutdownSec) * time.Second
	}
	if t.WaitNavMs > 0 {
		cfg.WaitNavDelay = time.Duration(t.WaitNavMs) * time.Millisecond
	}
}

func applySchedulerConfig(cfg *RuntimeConfig, s SchedulerFileConfig) {
	if s.Enabled != nil {
		cfg.Scheduler.Enabled = *s.Enabled
	}
	if s.Strategy != "" {
		cfg.Scheduler.Strategy = s.Strategy
	}
	if s.MaxQueueSize != nil {
		cfg.Scheduler.MaxQueueSize = *s.MaxQueueSize
	}
	if s.MaxPerAgent != nil {
		cfg.Scheduler.MaxPerAgent = *s.MaxPerAgent
	}
	if s.MaxInflight != nil {
		cfg.Scheduler.MaxInflight = *s.MaxInflight
	}
	if s.MaxPerAgentFlight != nil {
		cfg.Scheduler.MaxPerAgentFlight = *s.MaxPerAgentFlight
	}
	if s.ResultTTLSec != nil {
		cfg.Scheduler.ResultTTLSec = *s.ResultTTLSec
	}
	if s.WorkerCount != nil {
		cfg.Scheduler.WorkerCount = *s.WorkerCount
	}
	if s.MaxBatchSize != nil {
		cfg.Scheduler.MaxBatchSize = *s.MaxBatchSize
	}
}

func applyAutoSolverConfig(cfg *RuntimeConfig, a AutoSolverFileConfig) {
	if a.Enabled != nil {
		cfg.AutoSolver.Enabled = *a.Enabled
	}
	if a.AutoTrigger != nil {
		cfg.AutoSolver.AutoTrigger = *a.AutoTrigger
	}
	if a.TriggerOnNavigate != nil {
		cfg.AutoSolver.TriggerOnNavigate = *a.TriggerOnNavigate
	}
	if a.TriggerOnAction != nil {
		cfg.AutoSolver.TriggerOnAction = *a.TriggerOnAction
	}
	if a.MaxAttempts != nil && *a.MaxAttempts > 0 {
		cfg.AutoSolver.MaxAttempts = *a.MaxAttempts
	}
	if a.SolverTimeoutSec != nil && *a.SolverTimeoutSec > 0 {
		cfg.AutoSolver.SolverTimeoutSec = *a.SolverTimeoutSec
	}
	if a.RetryBaseDelayMs != nil && *a.RetryBaseDelayMs >= 0 {
		cfg.AutoSolver.RetryBaseDelayMs = *a.RetryBaseDelayMs
	}
	if a.RetryMaxDelayMs != nil && *a.RetryMaxDelayMs >= 0 {
		cfg.AutoSolver.RetryMaxDelayMs = *a.RetryMaxDelayMs
	}
	if len(a.Solvers) > 0 {
		cfg.AutoSolver.Solvers = append([]string(nil), a.Solvers...)
	}
	if a.LLMProvider != "" {
		cfg.AutoSolver.LLMProvider = a.LLMProvider
	}
	if a.LLMFallback != nil {
		cfg.AutoSolver.LLMFallback = *a.LLMFallback
	}
	cfg.AutoSolver.CapsolverKey = a.External.CapsolverKey
	cfg.AutoSolver.TwoCaptchaKey = a.External.TwoCaptchaKey
	cfg.AutoSolver.Credentials = AutoSolverCredentials{
		Login: AutoSolverLoginCreds{
			User:     a.Credentials.Login.User,
			Password: a.Credentials.Login.Password,
		},
		Signup: AutoSolverSignupCreds{
			Name:     a.Credentials.Signup.Name,
			Email:    a.Credentials.Signup.Email,
			Password: a.Credentials.Signup.Password,
		},
		Form: AutoSolverFormCreds{
			Field1: a.Credentials.Form.Field1,
			Field2: a.Credentials.Form.Field2,
			Email:  a.Credentials.Form.Email,
		},
	}
}

// ApplyFileConfigToRuntime merges file configuration into an existing runtime
// config and refreshes derived profile paths for long-running processes.
func ApplyFileConfigToRuntime(cfg *RuntimeConfig, fc *FileConfig) {
	if cfg == nil || fc == nil {
		return
	}

	EmitLoadDiagnostics(applyFileConfig(cfg, fc))
	finalizeProfileConfig(cfg)
}

func applyCloakBrowserConfigToRuntime(cfg *RuntimeConfig, cloak CloakBrowserConfig) {
	if cfg == nil {
		return
	}
	if cloak.FingerprintSeed != "" {
		cfg.Cloak.FingerprintSeed = cloak.FingerprintSeed
	}
	if cloak.Platform != "" {
		cfg.Cloak.Platform = cloak.Platform
	}
	if cloak.Locale != "" {
		cfg.Cloak.Locale = cloak.Locale
	}
	if cloak.Timezone != "" {
		cfg.Cloak.Timezone = cloak.Timezone
	}
	if cloak.WebRTCIP != "" {
		cfg.Cloak.WebRTCIP = cloak.WebRTCIP
	}
	if cloak.FontsDir != "" {
		cfg.Cloak.FontsDir = filepath.Clean(cloak.FontsDir)
	}
	if cloak.StorageQuotaMB != nil {
		cfg.Cloak.StorageQuotaMB = *cloak.StorageQuotaMB
	}
	if cloak.DisableDefaultStealthArgs != nil {
		cfg.Cloak.DisableDefaultStealthArgs = *cloak.DisableDefaultStealthArgs
	}
}
