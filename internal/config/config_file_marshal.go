package config

import (
	"encoding/json"
	"time"
)

func copyStringSlice(items []string) []string {
	if items == nil {
		return []string{}
	}
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items...)
}

func intPtrIfPositive(v int) *int {
	if v <= 0 {
		return nil
	}
	n := v
	return &n
}

// tabPolicyDefaultsFromRuntime emits a TabPolicyDefaults block when the runtime
// config carries any non-default tab-policy setting (lifecycle, close delay, or
// restore). Returns nil for a fully vanilla config so round-tripping doesn't
// introduce a noisy tabPolicy block.
func tabPolicyDefaultsFromRuntime(cfg *RuntimeConfig) *TabPolicyDefaults {
	if cfg == nil {
		return nil
	}
	hasLifecycle := cfg.TabLifecyclePolicy != "" &&
		(cfg.TabLifecyclePolicy != "close_idle" || cfg.TabCloseDelay != 5*time.Minute)
	hasRestore := cfg.TabRestore
	if !hasLifecycle && !hasRestore {
		return nil
	}
	out := &TabPolicyDefaults{}
	if hasLifecycle {
		out.Lifecycle = cfg.TabLifecyclePolicy
		if cfg.TabLifecyclePolicy == "close_idle" && cfg.TabCloseDelay > 0 {
			sec := int(cfg.TabCloseDelay / time.Second)
			out.CloseDelaySec = &sec
		}
	}
	if hasRestore {
		v := cfg.TabRestore
		out.Restore = &v
	}
	return out
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
			TabPolicy:         fc.InstanceDefaults.TabPolicy,
		},
		Security: securityConfigJSON{
			AllowEvaluate:          fc.Security.AllowEvaluate,
			AllowMacro:             fc.Security.AllowMacro,
			AllowScreencast:        fc.Security.AllowScreencast,
			AllowDownload:          fc.Security.AllowDownload,
			AllowedDomains:         effectiveSecurityAllowedDomains(fc.Security),
			DownloadAllowedDomains: copyStringSlice(fc.Security.DownloadAllowedDomains),
			DownloadMaxBytes:       fc.Security.DownloadMaxBytes,
			AllowUpload:            fc.Security.AllowUpload,
			AllowClipboard:         fc.Security.AllowClipboard,
			AllowStateExport:       fc.Security.AllowStateExport,
			StateEncryptionKey:     fc.Security.StateEncryptionKey,
			EnableActionGuards:     fc.Security.EnableActionGuards,
			UploadMaxRequestBytes:  fc.Security.UploadMaxRequestBytes,
			UploadMaxFiles:         fc.Security.UploadMaxFiles,
			UploadMaxFileBytes:     fc.Security.UploadMaxFileBytes,
			UploadMaxTotalBytes:    fc.Security.UploadMaxTotalBytes,
			MaxRedirects:           fc.Security.MaxRedirects,
			TrustedProxyCIDRs:      copyStringSlice(fc.Security.TrustedProxyCIDRs),
			TrustedResolveCIDRs:    copyStringSlice(fc.Security.TrustedResolveCIDRs),
			Attach: attachJSON{
				Enabled:      fc.Security.Attach.Enabled,
				AllowHosts:   copyStringSlice(fc.Security.Attach.AllowHosts),
				AllowSchemes: copyStringSlice(fc.Security.Attach.AllowSchemes),
			},
			IDPI: idpiConfigJSON{
				Enabled:         fc.Security.IDPI.Enabled,
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
				StateDir:       fc.Observability.Activity.StateDir,
				Events: activityEventsConfigJSON{
					Dashboard:    fc.Observability.Activity.Events.Dashboard,
					Server:       fc.Observability.Activity.Events.Server,
					Bridge:       fc.Observability.Activity.Events.Bridge,
					Orchestrator: fc.Observability.Activity.Events.Orchestrator,
					Scheduler:    fc.Observability.Activity.Events.Scheduler,
					MCP:          fc.Observability.Activity.Events.MCP,
					Other:        fc.Observability.Activity.Events.Other,
				},
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
			Enabled:           fc.AutoSolver.Enabled,
			AutoTrigger:       fc.AutoSolver.AutoTrigger,
			TriggerOnNavigate: fc.AutoSolver.TriggerOnNavigate,
			TriggerOnAction:   fc.AutoSolver.TriggerOnAction,
			MaxAttempts:       fc.AutoSolver.MaxAttempts,
			SolverTimeoutSec:  fc.AutoSolver.SolverTimeoutSec,
			RetryBaseDelayMs:  fc.AutoSolver.RetryBaseDelayMs,
			RetryMaxDelayMs:   fc.AutoSolver.RetryMaxDelayMs,
			Solvers:           copyStringSlice(fc.AutoSolver.Solvers),
			LLMProvider:       fc.AutoSolver.LLMProvider,
			LLMFallback:       fc.AutoSolver.LLMFallback,
			External: autoSolverExtConfigJSON{
				CapsolverKey:  fc.AutoSolver.External.CapsolverKey,
				TwoCaptchaKey: fc.AutoSolver.External.TwoCaptchaKey,
			},
			Credentials: autoSolverCredentialsConfigJSON{
				Login: autoSolverLoginConfigJSON{
					User:     fc.AutoSolver.Credentials.Login.User,
					Password: fc.AutoSolver.Credentials.Login.Password,
				},
				Signup: autoSolverSignupConfigJSON{
					Name:     fc.AutoSolver.Credentials.Signup.Name,
					Email:    fc.AutoSolver.Credentials.Signup.Email,
					Password: fc.AutoSolver.Credentials.Signup.Password,
				},
				Form: autoSolverFormConfigJSON{
					Field1: fc.AutoSolver.Credentials.Form.Field1,
					Field2: fc.AutoSolver.Credentials.Form.Field2,
					Email:  fc.AutoSolver.Credentials.Form.Email,
				},
			},
		},
	})
}

func (fc *FileConfig) UnmarshalJSON(data []byte) error {
	type rawFileConfig FileConfig
	tmp := rawFileConfig(*fc)
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*fc = FileConfig(tmp)
	NormalizeFileConfigAliasesFromJSON(fc, data)
	return nil
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
	allowStateExport := cfg.AllowStateExport
	enableActionGuards := cfg.EnableActionGuards
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
	activityDashboardEvents := cfg.Observability.Activity.Events.Dashboard
	activityServerEvents := cfg.Observability.Activity.Events.Server
	activityBridgeEvents := cfg.Observability.Activity.Events.Bridge
	activityOrchestratorEvents := cfg.Observability.Activity.Events.Orchestrator
	activitySchedulerEvents := cfg.Observability.Activity.Events.Scheduler
	activityMCPEvents := cfg.Observability.Activity.Events.MCP
	activityOtherEvents := cfg.Observability.Activity.Events.Other
	dashboardSessionPersist := cfg.Sessions.Dashboard.Persist
	dashboardSessionIdleSec := int(cfg.Sessions.Dashboard.IdleTimeout / time.Second)
	dashboardSessionMaxLifetimeSec := int(cfg.Sessions.Dashboard.MaxLifetime / time.Second)
	dashboardSessionElevationWindowSec := int(cfg.Sessions.Dashboard.ElevationWindow / time.Second)
	dashboardSessionPersistElevationAcrossRestart := cfg.Sessions.Dashboard.PersistElevationAcrossRestart
	dashboardSessionRequireElevation := cfg.Sessions.Dashboard.RequireElevation
	autoSolverEnabled := cfg.AutoSolver.Enabled
	autoSolverAutoTrigger := cfg.AutoSolver.AutoTrigger
	autoSolverTriggerOnNavigate := cfg.AutoSolver.TriggerOnNavigate
	autoSolverTriggerOnAction := cfg.AutoSolver.TriggerOnAction
	autoSolverMaxAttempts := cfg.AutoSolver.MaxAttempts
	autoSolverSolverTimeoutSec := cfg.AutoSolver.SolverTimeoutSec
	autoSolverRetryBaseDelayMs := cfg.AutoSolver.RetryBaseDelayMs
	autoSolverRetryMaxDelayMs := cfg.AutoSolver.RetryMaxDelayMs
	autoSolverLLMFallback := cfg.AutoSolver.LLMFallback

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
			TabPolicy:         tabPolicyDefaultsFromRuntime(cfg),
		},
		Security: SecurityConfig{
			AllowEvaluate:          &allowEvaluate,
			AllowMacro:             &allowMacro,
			AllowScreencast:        &allowScreencast,
			AllowDownload:          &allowDownload,
			AllowedDomains:         append([]string(nil), cfg.AllowedDomains...),
			DownloadAllowedDomains: downloadAllowedDomains,
			DownloadMaxBytes:       &downloadMaxBytes,
			AllowUpload:            &allowUpload,
			AllowClipboard:         &allowClipboard,
			AllowStateExport:       &allowStateExport,
			EnableActionGuards:     &enableActionGuards,
			UploadMaxRequestBytes:  &uploadMaxRequestBytes,
			UploadMaxFiles:         &uploadMaxFiles,
			UploadMaxFileBytes:     &uploadMaxFileBytes,
			UploadMaxTotalBytes:    &uploadMaxTotalBytes,
			MaxRedirects:           &maxRedirects,
			TrustedProxyCIDRs:      append([]string(nil), cfg.TrustedProxyCIDRs...),
			TrustedResolveCIDRs:    append([]string(nil), cfg.TrustedResolveCIDRs...),
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
				Events: ActivityEventsFileConfig{
					Dashboard:    &activityDashboardEvents,
					Server:       &activityServerEvents,
					Bridge:       &activityBridgeEvents,
					Orchestrator: &activityOrchestratorEvents,
					Scheduler:    &activitySchedulerEvents,
					MCP:          &activityMCPEvents,
					Other:        &activityOtherEvents,
				},
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
		AutoSolver: AutoSolverFileConfig{
			Enabled:           &autoSolverEnabled,
			AutoTrigger:       &autoSolverAutoTrigger,
			TriggerOnNavigate: &autoSolverTriggerOnNavigate,
			TriggerOnAction:   &autoSolverTriggerOnAction,
			MaxAttempts:       &autoSolverMaxAttempts,
			SolverTimeoutSec:  &autoSolverSolverTimeoutSec,
			RetryBaseDelayMs:  &autoSolverRetryBaseDelayMs,
			RetryMaxDelayMs:   &autoSolverRetryMaxDelayMs,
			Solvers:           copyStringSlice(cfg.AutoSolver.Solvers),
			LLMProvider:       cfg.AutoSolver.LLMProvider,
			LLMFallback:       &autoSolverLLMFallback,
			External: AutoSolverExtConf{
				CapsolverKey:  cfg.AutoSolver.CapsolverKey,
				TwoCaptchaKey: cfg.AutoSolver.TwoCaptchaKey,
			},
			Credentials: AutoSolverCredentialsConf{
				Login: AutoSolverLoginConf{
					User:     cfg.AutoSolver.Credentials.Login.User,
					Password: cfg.AutoSolver.Credentials.Login.Password,
				},
				Signup: AutoSolverSignupConf{
					Name:     cfg.AutoSolver.Credentials.Signup.Name,
					Email:    cfg.AutoSolver.Credentials.Signup.Email,
					Password: cfg.AutoSolver.Credentials.Signup.Password,
				},
				Form: AutoSolverFormConf{
					Field1: cfg.AutoSolver.Credentials.Form.Field1,
					Field2: cfg.AutoSolver.Credentials.Form.Field2,
					Email:  cfg.AutoSolver.Credentials.Form.Email,
				},
			},
		},
	}

	return fc
}
