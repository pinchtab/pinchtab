package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Load returns the RuntimeConfig with precedence: env vars > config file > defaults.
func Load() *RuntimeConfig {
	cfg := &RuntimeConfig{
		// Server defaults
		Bind:              "127.0.0.1",
		Port:              "9867",
		InstancePortStart: 9868,
		InstancePortEnd:   9968,
		Token:             os.Getenv("PINCHTAB_TOKEN"),
		StateDir:          userConfigDir(),
		CookieSecure:      nil,

		// Security defaults
		AllowEvaluate:          false,
		AllowMacro:             false,
		AllowScreencast:        false,
		AllowDownload:          false,
		DownloadAllowedDomains: nil,
		DownloadMaxBytes:       DefaultDownloadMaxBytes,
		AllowUpload:            false,
		UploadMaxRequestBytes:  DefaultUploadMaxRequestBytes,
		UploadMaxFiles:         DefaultUploadMaxFiles,
		UploadMaxFileBytes:     DefaultUploadMaxFileBytes,
		UploadMaxTotalBytes:    DefaultUploadMaxTotalBytes,
		MaxRedirects:           -1, // Unlimited by default; set to N to limit redirect hops

		// Browser / instance defaults
		Headless:          true,
		NoRestore:         false,
		ProfileDir:        "",
		ProfilesBaseDir:   "",
		DefaultProfile:    "default",
		ChromeVersion:     "144.0.7559.133",
		Timezone:          "",
		BlockImages:       false,
		BlockMedia:        false,
		BlockAds:          false,
		MaxTabs:           20,
		MaxParallelTabs:   0,
		ChromeBinary:      "", // Set via config.json only
		ChromeExtraFlags:  "",
		ExtensionPaths:    nil,
		UserAgent:         "",
		NoAnimations:      false,
		StealthLevel:      "light",
		TabEvictionPolicy: "close_lru",

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
		AttachAllowSchemes: []string{"ws", "wss"},

		// IDPI defaults
		IDPI: IDPIConfig{
			Enabled:        true,
			AllowedDomains: append([]string(nil), defaultLocalAllowedDomains...),
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
				RetentionDays:  1,
				StateDir:       "",
			},
		},

		// Dashboard session defaults
		Sessions: SessionsRuntimeConfig{
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
			Enabled:     false,
			MaxAttempts: 8,
			Solvers:     []string{"cloudflare", "semantic", "capsolver", "twocaptcha"},
			LLMFallback: false,
		},
	}
	finalizeProfileConfig(cfg)

	// Load config file (supports both legacy flat and new nested format)
	configPath := envOr("PINCHTAB_CONFIG", filepath.Join(userConfigDir(), "config.json"))

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

	return cfg
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
	cfg.AttachAllowHosts = append([]string(nil), fc.Security.Attach.AllowHosts...)
	cfg.AttachAllowSchemes = append([]string(nil), fc.Security.Attach.AllowSchemes...)
	cfg.TrustedProxyCIDRs = append([]string(nil), fc.Security.TrustedProxyCIDRs...)
	// IDPI – copy the whole struct; individual fields have safe zero-value defaults.
	cfg.IDPI = fc.Security.IDPI
	if fc.Observability.Activity.Enabled != nil {
		cfg.Observability.Activity.Enabled = *fc.Observability.Activity.Enabled
	}
	if fc.Observability.Activity.SessionIdleSec != nil {
		cfg.Observability.Activity.SessionIdleSec = *fc.Observability.Activity.SessionIdleSec
	}
	if fc.Observability.Activity.RetentionDays != nil {
		cfg.Observability.Activity.RetentionDays = *fc.Observability.Activity.RetentionDays
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

	// Browser
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
	if len(fc.Browser.ExtensionPaths) > 0 {
		cfg.ExtensionPaths = fc.Browser.ExtensionPaths
	}

	// Instance defaults
	if fc.InstanceDefaults.Mode != "" {
		cfg.Headless = modeToHeadless(fc.InstanceDefaults.Mode, cfg.Headless)
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
	if fc.InstanceDefaults.StealthLevel != "" {
		cfg.StealthLevel = fc.InstanceDefaults.StealthLevel
	}
	if fc.InstanceDefaults.TabEvictionPolicy != "" {
		cfg.TabEvictionPolicy = fc.InstanceDefaults.TabEvictionPolicy
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
	if fc.AutoSolver.MaxAttempts != nil && *fc.AutoSolver.MaxAttempts > 0 {
		cfg.AutoSolver.MaxAttempts = *fc.AutoSolver.MaxAttempts
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
