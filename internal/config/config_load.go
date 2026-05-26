package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// Load returns the RuntimeConfig with precedence: env vars > config file > defaults.
func Load() *RuntimeConfig {
	cfg := &RuntimeConfig{
		// Server defaults
		Bind:              "127.0.0.1",
		Port:              defaultPort,
		InstancePortStart: 9868,
		InstancePortEnd:   9968,
		Token:             os.Getenv("PINCHTAB_TOKEN"),
		StateDir:          userConfigDir(),
		CookieSecure:      nil,

		// Security defaults
		AllowEvaluate:             false,
		AllowMacro:                false,
		AllowScreencast:           false,
		AllowDownload:             false,
		AllowCookies:              false,
		AllowNetworkIntercept:     false,
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

		// Browser / instance defaults
		Headless:           true,
		NoRestore:          false,
		ProfileDir:         "",
		ProfilesBaseDir:    "",
		DefaultProfile:     "default",
		ChromeVersion:      "144.0.7559.133",
		Timezone:           "",
		BlockImages:        false,
		BlockMedia:         false,
		BlockAds:           false,
		MaxTabs:            20,
		MaxParallelTabs:    0,
		DefaultBrowser:     BrowserChrome,
		ChromeBinary:       "", // Set via config.json only
		ChromeExtraFlags:   "",
		Cloak:              CloakBrowserRuntimeConfig{DisableDefaultStealthArgs: true},
		ExtensionPaths:     []string{defaultExtensionsDir(userConfigDir())},
		UserAgent:          "",
		NoAnimations:       false,
		Humanize:           false,
		StealthLevel:       "light",
		TabEvictionPolicy:  "close_lru",
		TabLifecyclePolicy: "keep",
		TabCloseDelay:      5 * time.Minute,
		TabRestore:         false,

		// Timeout defaults
		ActionTimeout:   30 * time.Second,
		NavigateTimeout: 60 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		WaitNavDelay:    1 * time.Second,

		// Orchestrator defaults
		Strategy:           "always-on",
		AllocationPolicy:   "fcfs",
		RestartMaxRestarts: 20,
		RestartInitBackoff: 2 * time.Second,
		RestartMaxBackoff:  60 * time.Second,
		RestartStableAfter: 5 * time.Minute,

		// Attach defaults
		AttachEnabled:      false,
		AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
		AttachAllowSchemes: []string{"ws", "wss", "http", "https"},

		// IDPI defaults
		IDPI: IDPIConfig{
			Enabled:        true,
			StrictMode:     true,
			ScanContent:    true,
			WrapContent:    true,
			ScanTimeoutSec: 5,
		},

		// Engine default (set via config.json only)
		Engine: "chrome",

		// Observability defaults
		Observability: ObservabilityConfig{
			Activity: ActivityConfig{
				Enabled:        true,
				SessionIdleSec: 1800,
				RetentionDays:  30,
				StateDir:       "",
			},
		},

		// Session defaults
		Sessions: SessionsRuntimeConfig{
			Agent: AgentSessionRuntimeConfig{
				Enabled:     true,
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

		// AutoSolver defaults (disabled by default)
		AutoSolver: AutoSolverConfig{
			Enabled:           false,
			AutoTrigger:       true,
			TriggerOnNavigate: true,
			TriggerOnAction:   true,
			MaxAttempts:       8,
			SolverTimeoutSec:  30,
			RetryBaseDelayMs:  500,
			RetryMaxDelayMs:   10000,
			Solvers:           []string{"cloudflare", "semantic", "capsolver", "twocaptcha"},
			LLMFallback:       false,
		},
	}
	finalizeProfileConfig(cfg)

	// Load config file (supports both legacy flat and new nested format)
	defaultConfigPath := filepath.Join(userConfigDir(), "config.json")
	configPath := envOr("PINCHTAB_CONFIG", defaultConfigPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read config file", "path", configPath, "error", err)
		}
		return cfg
	}

	slog.Debug("loading config file", "path", configPath)

	var fc *FileConfig

	if isLegacyConfig(data) {
		var lc legacyFileConfig
		if err := json.Unmarshal(data, &lc); err != nil {
			slog.Warn("failed to parse legacy config", "path", configPath, "error", err)
			return cfg
		}
		fc = convertLegacyConfig(&lc)
		slog.Info("loaded legacy flat config, consider migrating to nested format", "path", configPath)
	} else {
		fc = &FileConfig{}
		if err := json.Unmarshal(data, fc); err != nil {
			slog.Warn("failed to parse config", "path", configPath, "error", err)
			return cfg
		}
		// Warn about unrecognized fields (non-fatal, config still loads).
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if ufErr := dec.Decode(&FileConfig{}); ufErr != nil {
			slog.Warn("config has unrecognized fields that will be ignored", "path", configPath, "error", ufErr)
		}
	}

	// Validate file config and log warnings
	if errs := ValidateFileConfig(fc); len(errs) > 0 {
		for _, e := range errs {
			slog.Warn("config validation error", "path", configPath, "error", e)
		}
	}

	// Apply file config (only if env var NOT set)
	applyFileConfig(cfg, fc)
	finalizeProfileConfig(cfg)

	if cfg.Port == "" {
		slog.Error("server port is not configured — set server.port in config.json")
		os.Exit(1)
	}

	return cfg
}

// ConfigFileStatus reports on-disk config state without invoking Load (used by `pinchtab doctor`).
type ConfigFileStatus struct {
	Path        string
	DefaultPath string
	EnvOverride bool
	Found       bool
	ParseErr    error
}

// InspectConfigFile reports config-file load status with no side effects.
func InspectConfigFile() ConfigFileStatus {
	defaultPath := filepath.Join(userConfigDir(), "config.json")
	path := envOr("PINCHTAB_CONFIG", defaultPath)
	status := ConfigFileStatus{
		Path:        path,
		DefaultPath: defaultPath,
		EnvOverride: os.Getenv("PINCHTAB_CONFIG") != "",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return status
	}
	status.Found = true

	if isLegacyConfig(data) {
		var lc legacyFileConfig
		if err := json.Unmarshal(data, &lc); err != nil {
			status.ParseErr = err
		}
		return status
	}
	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		status.ParseErr = err
	}
	return status
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

func applyFileConfig(cfg *RuntimeConfig, fc *FileConfig) {
	// Server
	if fc.Server.Port != "" {
		cfg.Port = fc.Server.Port
	}
	if fc.Server.Bind != "" {
		cfg.Bind = fc.Server.Bind
	}
	if os.Getenv("PINCHTAB_TOKEN") == "" {
		cfg.Token = fc.Server.Token
	}
	if fc.Server.StateDir != "" {
		cfg.StateDir = fc.Server.StateDir
	}
	if fc.Server.Engine != "" {
		cfg.Engine = fc.Server.Engine
	}
	if fc.Server.NetworkBufferSize != nil && *fc.Server.NetworkBufferSize > 0 {
		cfg.NetworkBufferSize = ClampNetworkBufferSize(*fc.Server.NetworkBufferSize)
	}
	if fc.Server.RetainNetworkBodies != nil {
		cfg.RetainNetworkBodies = *fc.Server.RetainNetworkBodies
	}
	if fc.Server.RetainNetworkBodyMaxBytes != nil && *fc.Server.RetainNetworkBodyMaxBytes >= 0 {
		cfg.RetainNetworkBodyMaxBytes = *fc.Server.RetainNetworkBodyMaxBytes
	}
	if fc.Server.TrustProxyHeaders != nil {
		cfg.TrustProxyHeaders = *fc.Server.TrustProxyHeaders
	}
	cfg.CookieSecure = fc.Server.CookieSecure
	// Security
	if fc.Security.AllowEvaluate != nil {
		cfg.AllowEvaluate = *fc.Security.AllowEvaluate
	}
	if fc.Security.AllowMacro != nil {
		cfg.AllowMacro = *fc.Security.AllowMacro
	}
	if fc.Security.AllowScreencast != nil {
		cfg.AllowScreencast = *fc.Security.AllowScreencast
	}
	if fc.Security.AllowDownload != nil {
		cfg.AllowDownload = *fc.Security.AllowDownload
	}
	if fc.Security.AllowCookies != nil {
		cfg.AllowCookies = *fc.Security.AllowCookies
	}
	if fc.Security.AllowNetworkIntercept != nil {
		cfg.AllowNetworkIntercept = *fc.Security.AllowNetworkIntercept
	}
	cfg.DownloadAllowedDomains = append([]string(nil), fc.Security.DownloadAllowedDomains...)
	if fc.Security.DownloadMaxBytes != nil {
		cfg.DownloadMaxBytes = clampPositiveLimit(*fc.Security.DownloadMaxBytes, DefaultDownloadMaxBytes, MaxDownloadMaxBytes)
	}
	if fc.Security.AllowUpload != nil {
		cfg.AllowUpload = *fc.Security.AllowUpload
	}
	if fc.Security.AllowClipboard != nil {
		cfg.AllowClipboard = *fc.Security.AllowClipboard
	}
	if fc.Security.AllowStateExport != nil {
		cfg.AllowStateExport = *fc.Security.AllowStateExport
	}
	if fc.Security.StateEncryptionKey != nil {
		cfg.StateEncryptionKey = *fc.Security.StateEncryptionKey
	}
	if fc.Security.EnableActionGuards != nil {
		cfg.EnableActionGuards = *fc.Security.EnableActionGuards
	}
	if fc.Security.UploadMaxRequestBytes != nil {
		cfg.UploadMaxRequestBytes = clampPositiveLimit(*fc.Security.UploadMaxRequestBytes, DefaultUploadMaxRequestBytes, MaxUploadMaxRequestBytes)
	}
	if fc.Security.UploadMaxFiles != nil {
		cfg.UploadMaxFiles = clampPositiveLimit(*fc.Security.UploadMaxFiles, DefaultUploadMaxFiles, MaxUploadMaxFiles)
	}
	if fc.Security.UploadMaxFileBytes != nil {
		cfg.UploadMaxFileBytes = clampPositiveLimit(*fc.Security.UploadMaxFileBytes, DefaultUploadMaxFileBytes, MaxUploadMaxFileBytes)
	}
	if fc.Security.UploadMaxTotalBytes != nil {
		cfg.UploadMaxTotalBytes = clampPositiveLimit(*fc.Security.UploadMaxTotalBytes, DefaultUploadMaxTotalBytes, MaxUploadMaxTotalBytes)
	}
	if fc.Security.MaxRedirects != nil {
		cfg.MaxRedirects = *fc.Security.MaxRedirects
	}
	if fc.Security.Attach.Enabled != nil {
		cfg.AttachEnabled = *fc.Security.Attach.Enabled
	}
	if fc.Security.Attach.ForwardProxyAuth != nil {
		cfg.AttachForwardProxyAuth = *fc.Security.Attach.ForwardProxyAuth
	}
	cfg.AttachAllowHosts = append([]string(nil), fc.Security.Attach.AllowHosts...)
	cfg.AttachAllowSchemes = append([]string(nil), fc.Security.Attach.AllowSchemes...)
	cfg.TrustedProxyCIDRs = append([]string(nil), fc.Security.TrustedProxyCIDRs...)
	cfg.TrustedResolveCIDRs = append([]string(nil), fc.Security.TrustedResolveCIDRs...)
	if fc.Security.TrustLoopbackProxy != nil {
		cfg.TrustLoopbackProxy = *fc.Security.TrustLoopbackProxy
	}
	// IDPI – copy the whole struct; individual fields have safe zero-value defaults.
	cfg.IDPI = fc.Security.IDPI
	cfg.AllowedDomains = effectiveSecurityAllowedDomains(fc.Security)
	if fc.Observability.Activity.Enabled != nil {
		cfg.Observability.Activity.Enabled = *fc.Observability.Activity.Enabled
	}
	if fc.Observability.Activity.SessionIdleSec != nil {
		cfg.Observability.Activity.SessionIdleSec = *fc.Observability.Activity.SessionIdleSec
	}
	if fc.Observability.Activity.RetentionDays != nil {
		cfg.Observability.Activity.RetentionDays = *fc.Observability.Activity.RetentionDays
	}
	if fc.Observability.Activity.Events.Dashboard != nil {
		cfg.Observability.Activity.Events.Dashboard = *fc.Observability.Activity.Events.Dashboard
	}
	if fc.Observability.Activity.Events.Server != nil {
		cfg.Observability.Activity.Events.Server = *fc.Observability.Activity.Events.Server
	}
	if fc.Observability.Activity.Events.Bridge != nil {
		cfg.Observability.Activity.Events.Bridge = *fc.Observability.Activity.Events.Bridge
	}
	if fc.Observability.Activity.Events.Orchestrator != nil {
		cfg.Observability.Activity.Events.Orchestrator = *fc.Observability.Activity.Events.Orchestrator
	}
	if fc.Observability.Activity.Events.Scheduler != nil {
		cfg.Observability.Activity.Events.Scheduler = *fc.Observability.Activity.Events.Scheduler
	}
	if fc.Observability.Activity.Events.MCP != nil {
		cfg.Observability.Activity.Events.MCP = *fc.Observability.Activity.Events.MCP
	}
	if fc.Observability.Activity.Events.Other != nil {
		cfg.Observability.Activity.Events.Other = *fc.Observability.Activity.Events.Other
	}
	if fc.Sessions.Dashboard.Persist != nil {
		cfg.Sessions.Dashboard.Persist = *fc.Sessions.Dashboard.Persist
	}
	if fc.Sessions.Dashboard.IdleTimeoutSec != nil && *fc.Sessions.Dashboard.IdleTimeoutSec > 0 {
		cfg.Sessions.Dashboard.IdleTimeout = time.Duration(*fc.Sessions.Dashboard.IdleTimeoutSec) * time.Second
	}
	if fc.Sessions.Dashboard.MaxLifetimeSec != nil && *fc.Sessions.Dashboard.MaxLifetimeSec > 0 {
		cfg.Sessions.Dashboard.MaxLifetime = time.Duration(*fc.Sessions.Dashboard.MaxLifetimeSec) * time.Second
	}
	if fc.Sessions.Dashboard.ElevationWindowSec != nil && *fc.Sessions.Dashboard.ElevationWindowSec > 0 {
		cfg.Sessions.Dashboard.ElevationWindow = time.Duration(*fc.Sessions.Dashboard.ElevationWindowSec) * time.Second
	}
	if fc.Sessions.Dashboard.PersistElevationAcrossRestart != nil {
		cfg.Sessions.Dashboard.PersistElevationAcrossRestart = *fc.Sessions.Dashboard.PersistElevationAcrossRestart
	}
	if fc.Sessions.Dashboard.RequireElevation != nil {
		cfg.Sessions.Dashboard.RequireElevation = *fc.Sessions.Dashboard.RequireElevation
	}

	// Agent sessions
	if fc.Sessions.Agent.Enabled != nil {
		cfg.Sessions.Agent.Enabled = *fc.Sessions.Agent.Enabled
	}
	if fc.Sessions.Agent.Mode != "" {
		cfg.Sessions.Agent.Mode = fc.Sessions.Agent.Mode
	}
	if fc.Sessions.Agent.IdleTimeoutSec != nil && *fc.Sessions.Agent.IdleTimeoutSec > 0 {
		cfg.Sessions.Agent.IdleTimeout = time.Duration(*fc.Sessions.Agent.IdleTimeoutSec) * time.Second
	}
	if fc.Sessions.Agent.MaxLifetimeSec != nil && *fc.Sessions.Agent.MaxLifetimeSec > 0 {
		cfg.Sessions.Agent.MaxLifetime = time.Duration(*fc.Sessions.Agent.MaxLifetimeSec) * time.Second
	}

	// Migration shim must run before consuming legacy provider/binary/cloak fields below.
	if synthesized, conflict := migrateLegacyBrowserConfig(&fc.Browser); conflict {
		slog.Warn("config has both browser.targets and legacy browser.provider/binary/cloak set; explicit targets win, legacy fields ignored for target resolution")
	} else if synthesized {
		slog.Debug("migrated legacy browser config into browser.targets.default")
	}
	if len(fc.Browser.Targets) > 0 {
		cfg.Targets = cloneBrowserTargetsConfig(fc.Browser.Targets)
		cfg.DefaultTarget = fc.Browser.DefaultTarget
		cfg.FallbackOrder = append([]string(nil), fc.Browser.FallbackOrder...)
	}

	// Resolve the effective browser provider: browsers.default is the
	// authoritative source; the deprecated server.engine field is a fallback.
	// browser.provider is no longer supported (rejected at validation time).
	if fc.Browsers.Default != "" {
		cfg.DefaultBrowser = fc.Browsers.Default
		if fc.Server.Engine != "" {
			slog.Warn("both server.engine and browsers.default are set; browsers.default takes precedence, server.engine is ignored",
				"engine", fc.Server.Engine, "browsers.default", fc.Browsers.Default)
		}
	} else if fc.Server.Engine != "" {
		// Deprecated: migrate server.engine → DefaultBrowser.
		switch fc.Server.Engine {
		case "chrome":
			cfg.DefaultBrowser = BrowserChrome
		case "lite", "auto":
			cfg.DefaultBrowser = BrowserGhostChrome
		default:
			cfg.DefaultBrowser = BrowserChrome
		}
		slog.Warn("server.engine is deprecated; migrate to browsers.default in config.json",
			"engine", fc.Server.Engine, "browsers.default", cfg.DefaultBrowser)
	} else {
		cfg.DefaultBrowser = "chrome"
	}

	// Apply native-stealth default when the winning provider supports it.
	if b, ok := browsers.Get(strings.ToLower(cfg.DefaultBrowser)); ok && b.Capabilities().Has(browsers.CapNativeStealth) && fc.Browser.Cloak.DisableDefaultStealthArgs == nil {
		cfg.Cloak.DisableDefaultStealthArgs = true
	}

	if fc.Browser.ChromeVersion != "" {
		cfg.ChromeVersion = fc.Browser.ChromeVersion
	}
	if fc.Browser.ChromeBinary != "" {
		cfg.ChromeBinary = fc.Browser.ChromeBinary
	}
	if fc.Browser.ChromeDebugPort != nil && *fc.Browser.ChromeDebugPort > 0 {
		cfg.ChromeDebugPort = *fc.Browser.ChromeDebugPort
	}
	if fc.Browser.ChromeExtraFlags != "" {
		cfg.ChromeExtraFlags = SanitizeChromeExtraFlags(fc.Browser.ChromeExtraFlags)
	}
	applyCloakBrowserConfigToRuntime(cfg, fc.Browser.Cloak)
	if !fc.Browser.Proxy.IsZero() {
		cfg.Proxy = BrowserProxyConfig{
			Server:     fc.Browser.Proxy.Server,
			BypassList: append([]string(nil), fc.Browser.Proxy.BypassList...),
			Username:   fc.Browser.Proxy.Username,
			Password:   fc.Browser.Proxy.Password,
		}
		if fc.Browser.Proxy.Geo != nil {
			geoCopy := *fc.Browser.Proxy.Geo
			cfg.Proxy.Geo = &geoCopy
		}
	}
	if fc.Browser.ExtensionPaths != nil {
		cfg.ExtensionPaths = append([]string(nil), fc.Browser.ExtensionPaths...)
	}

	// Process browsers block — available list
	if len(fc.Browsers.Available) > 0 {
		cfg.BrowsersAvailable = make([]string, len(fc.Browsers.Available))
		copy(cfg.BrowsersAvailable, fc.Browsers.Available)
	} else if cfg.DefaultBrowser != "" {
		cfg.BrowsersAvailable = []string{cfg.DefaultBrowser}
	} else {
		cfg.BrowsersAvailable = []string{"chrome"}
	}

	// Instance defaults — resolve headless bool into mode string.
	if fc.InstanceDefaults.Headless != nil && fc.InstanceDefaults.Mode == "" {
		if *fc.InstanceDefaults.Headless {
			fc.InstanceDefaults.Mode = "headless"
		} else {
			fc.InstanceDefaults.Mode = "headed"
		}
	}
	if fc.InstanceDefaults.Mode != "" {
		cfg.Headless = modeToHeadless(fc.InstanceDefaults.Mode, cfg.Headless)
		cfg.HeadlessSet = true
	}
	if fc.InstanceDefaults.NoRestore != nil {
		cfg.NoRestore = *fc.InstanceDefaults.NoRestore
	}
	if fc.InstanceDefaults.Timezone != "" {
		cfg.Timezone = fc.InstanceDefaults.Timezone
	}
	if fc.InstanceDefaults.BlockImages != nil {
		cfg.BlockImages = *fc.InstanceDefaults.BlockImages
	}
	if fc.InstanceDefaults.BlockMedia != nil {
		cfg.BlockMedia = *fc.InstanceDefaults.BlockMedia
	}
	if fc.InstanceDefaults.BlockAds != nil {
		cfg.BlockAds = *fc.InstanceDefaults.BlockAds
	}
	if fc.InstanceDefaults.MaxTabs != nil {
		cfg.MaxTabs = *fc.InstanceDefaults.MaxTabs
	}
	if fc.InstanceDefaults.MaxParallelTabs != nil {
		cfg.MaxParallelTabs = *fc.InstanceDefaults.MaxParallelTabs
	}
	if fc.InstanceDefaults.UserAgent != "" {
		cfg.UserAgent = fc.InstanceDefaults.UserAgent
	}
	if fc.InstanceDefaults.NoAnimations != nil {
		cfg.NoAnimations = *fc.InstanceDefaults.NoAnimations
	}
	if fc.InstanceDefaults.Humanize != nil {
		cfg.Humanize = *fc.InstanceDefaults.Humanize
	}
	if fc.InstanceDefaults.StealthLevel != "" {
		cfg.StealthLevel = fc.InstanceDefaults.StealthLevel
	}
	if fc.InstanceDefaults.TabEvictionPolicy != "" {
		cfg.TabEvictionPolicy = fc.InstanceDefaults.TabEvictionPolicy
	}
	if tp := fc.InstanceDefaults.TabPolicy; tp != nil {
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
	// Clamp to a sane minimum to avoid races between handler return and timer fire.
	if cfg.TabLifecyclePolicy == "close_idle" && cfg.TabCloseDelay < time.Second {
		cfg.TabCloseDelay = time.Second
	}
	if fc.InstanceDefaults.DialogAutoAccept != nil {
		cfg.DialogAutoAccept = *fc.InstanceDefaults.DialogAutoAccept
	}

	// Profiles
	if fc.Profiles.BaseDir != "" {
		cfg.ProfilesBaseDir = fc.Profiles.BaseDir
	}
	if fc.Profiles.DefaultProfile != "" {
		cfg.DefaultProfile = fc.Profiles.DefaultProfile
	}
	cfg.ProfileDir = ""

	// Multi-instance
	if fc.MultiInstance.Strategy != "" {
		cfg.Strategy = fc.MultiInstance.Strategy
	}
	if fc.MultiInstance.AllocationPolicy != "" {
		cfg.AllocationPolicy = fc.MultiInstance.AllocationPolicy
	}
	if fc.MultiInstance.InstancePortStart != nil {
		cfg.InstancePortStart = *fc.MultiInstance.InstancePortStart
	}
	if fc.MultiInstance.InstancePortEnd != nil {
		cfg.InstancePortEnd = *fc.MultiInstance.InstancePortEnd
	}
	// Restart
	if fc.MultiInstance.Restart.MaxRestarts != nil {
		cfg.RestartMaxRestarts = *fc.MultiInstance.Restart.MaxRestarts
	}
	if fc.MultiInstance.Restart.InitBackoffSec != nil {
		cfg.RestartInitBackoff = time.Duration(*fc.MultiInstance.Restart.InitBackoffSec) * time.Second
	}
	if fc.MultiInstance.Restart.MaxBackoffSec != nil {
		cfg.RestartMaxBackoff = time.Duration(*fc.MultiInstance.Restart.MaxBackoffSec) * time.Second
	}
	if fc.MultiInstance.Restart.StableAfterSec != nil {
		cfg.RestartStableAfter = time.Duration(*fc.MultiInstance.Restart.StableAfterSec) * time.Second
	}

	// Attach
	if fc.Security.Attach.Enabled != nil {
		cfg.AttachEnabled = *fc.Security.Attach.Enabled
	}
	if fc.Security.Attach.ForwardProxyAuth != nil {
		cfg.AttachForwardProxyAuth = *fc.Security.Attach.ForwardProxyAuth
	}
	if len(fc.Security.Attach.AllowHosts) > 0 {
		cfg.AttachAllowHosts = append([]string(nil), fc.Security.Attach.AllowHosts...)
	}
	if len(fc.Security.Attach.AllowSchemes) > 0 {
		cfg.AttachAllowSchemes = append([]string(nil), fc.Security.Attach.AllowSchemes...)
	}

	// Timeouts
	if fc.Timeouts.ActionSec > 0 {
		cfg.ActionTimeout = time.Duration(fc.Timeouts.ActionSec) * time.Second
	}
	if fc.Timeouts.NavigateSec > 0 {
		cfg.NavigateTimeout = time.Duration(fc.Timeouts.NavigateSec) * time.Second
	}
	if fc.Timeouts.ShutdownSec > 0 {
		cfg.ShutdownTimeout = time.Duration(fc.Timeouts.ShutdownSec) * time.Second
	}
	if fc.Timeouts.WaitNavMs > 0 {
		cfg.WaitNavDelay = time.Duration(fc.Timeouts.WaitNavMs) * time.Millisecond
	}

	// Scheduler
	if fc.Scheduler.Enabled != nil {
		cfg.Scheduler.Enabled = *fc.Scheduler.Enabled
	}
	if fc.Scheduler.Strategy != "" {
		cfg.Scheduler.Strategy = fc.Scheduler.Strategy
	}
	if fc.Scheduler.MaxQueueSize != nil {
		cfg.Scheduler.MaxQueueSize = *fc.Scheduler.MaxQueueSize
	}
	if fc.Scheduler.MaxPerAgent != nil {
		cfg.Scheduler.MaxPerAgent = *fc.Scheduler.MaxPerAgent
	}
	if fc.Scheduler.MaxInflight != nil {
		cfg.Scheduler.MaxInflight = *fc.Scheduler.MaxInflight
	}
	if fc.Scheduler.MaxPerAgentFlight != nil {
		cfg.Scheduler.MaxPerAgentFlight = *fc.Scheduler.MaxPerAgentFlight
	}
	if fc.Scheduler.ResultTTLSec != nil {
		cfg.Scheduler.ResultTTLSec = *fc.Scheduler.ResultTTLSec
	}
	if fc.Scheduler.WorkerCount != nil {
		cfg.Scheduler.WorkerCount = *fc.Scheduler.WorkerCount
	}

	// AutoSolver
	if fc.AutoSolver.Enabled != nil {
		cfg.AutoSolver.Enabled = *fc.AutoSolver.Enabled
	}
	if fc.AutoSolver.AutoTrigger != nil {
		cfg.AutoSolver.AutoTrigger = *fc.AutoSolver.AutoTrigger
	}
	if fc.AutoSolver.TriggerOnNavigate != nil {
		cfg.AutoSolver.TriggerOnNavigate = *fc.AutoSolver.TriggerOnNavigate
	}
	if fc.AutoSolver.TriggerOnAction != nil {
		cfg.AutoSolver.TriggerOnAction = *fc.AutoSolver.TriggerOnAction
	}
	if fc.AutoSolver.MaxAttempts != nil && *fc.AutoSolver.MaxAttempts > 0 {
		cfg.AutoSolver.MaxAttempts = *fc.AutoSolver.MaxAttempts
	}
	if fc.AutoSolver.SolverTimeoutSec != nil && *fc.AutoSolver.SolverTimeoutSec > 0 {
		cfg.AutoSolver.SolverTimeoutSec = *fc.AutoSolver.SolverTimeoutSec
	}
	if fc.AutoSolver.RetryBaseDelayMs != nil && *fc.AutoSolver.RetryBaseDelayMs >= 0 {
		cfg.AutoSolver.RetryBaseDelayMs = *fc.AutoSolver.RetryBaseDelayMs
	}
	if fc.AutoSolver.RetryMaxDelayMs != nil && *fc.AutoSolver.RetryMaxDelayMs >= 0 {
		cfg.AutoSolver.RetryMaxDelayMs = *fc.AutoSolver.RetryMaxDelayMs
	}
	if len(fc.AutoSolver.Solvers) > 0 {
		cfg.AutoSolver.Solvers = append([]string(nil), fc.AutoSolver.Solvers...)
	}
	if fc.AutoSolver.LLMProvider != "" {
		cfg.AutoSolver.LLMProvider = fc.AutoSolver.LLMProvider
	}
	if fc.AutoSolver.LLMFallback != nil {
		cfg.AutoSolver.LLMFallback = *fc.AutoSolver.LLMFallback
	}
	cfg.AutoSolver.CapsolverKey = fc.AutoSolver.External.CapsolverKey
	cfg.AutoSolver.TwoCaptchaKey = fc.AutoSolver.External.TwoCaptchaKey
	cfg.AutoSolver.Credentials = AutoSolverCredentials{
		Login: AutoSolverLoginCreds{
			User:     fc.AutoSolver.Credentials.Login.User,
			Password: fc.AutoSolver.Credentials.Login.Password,
		},
		Signup: AutoSolverSignupCreds{
			Name:     fc.AutoSolver.Credentials.Signup.Name,
			Email:    fc.AutoSolver.Credentials.Signup.Email,
			Password: fc.AutoSolver.Credentials.Signup.Password,
		},
		Form: AutoSolverFormCreds{
			Field1: fc.AutoSolver.Credentials.Form.Field1,
			Field2: fc.AutoSolver.Credentials.Form.Field2,
			Email:  fc.AutoSolver.Credentials.Form.Email,
		},
	}
}

// ApplyFileConfigToRuntime merges file configuration into an existing runtime
// config and refreshes derived profile paths for long-running processes.
func ApplyFileConfigToRuntime(cfg *RuntimeConfig, fc *FileConfig) {
	if cfg == nil || fc == nil {
		return
	}

	applyFileConfig(cfg, fc)
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
