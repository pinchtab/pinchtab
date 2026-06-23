package config

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
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

func intPtrIfNonNegative(v int) *int {
	if v < 0 {
		return nil
	}
	n := v
	return &n
}

func boolPtrValue(v bool) *bool {
	b := v
	return &b
}

func cloakBrowserConfigJSONFromFile(c CloakBrowserConfig) *cloakBrowserConfigJSON {
	if !hasCloakBrowserConfig(c) {
		return nil
	}
	return &cloakBrowserConfigJSON{
		FingerprintSeed:           c.FingerprintSeed,
		Platform:                  c.Platform,
		Locale:                    c.Locale,
		Timezone:                  c.Timezone,
		WebRTCIP:                  c.WebRTCIP,
		FontsDir:                  c.FontsDir,
		StorageQuotaMB:            c.StorageQuotaMB,
		DisableDefaultStealthArgs: c.DisableDefaultStealthArgs,
	}
}

// browserProxyJSONFromFile returns nil when proxy is disabled so omitempty drops the field.
func browserProxyJSONFromFile(p BrowserProxyConfig) *BrowserProxyConfig {
	if p.IsZero() {
		return nil
	}
	out := BrowserProxyConfig{
		Server:   p.Server,
		Username: p.Username,
		Password: p.Password,
	}
	if len(p.BypassList) > 0 {
		out.BypassList = append([]string(nil), p.BypassList...)
	}
	if p.Geo != nil && !p.Geo.IsZero() {
		geoCopy := *p.Geo
		out.Geo = &geoCopy
	}
	return &out
}

func cloakBrowserConfigFromRuntime(cfg *RuntimeConfig) CloakBrowserConfig {
	if cfg == nil {
		return CloakBrowserConfig{}
	}
	c := cfg.Cloak
	providerHasNativeStealth := false
	if b, ok := browsers.Get(strings.ToLower(cfg.DefaultBrowser)); ok {
		providerHasNativeStealth = b.Capabilities().Has(browsers.CapNativeStealth)
	}
	hasRuntimeCloak := providerHasNativeStealth ||
		c.FingerprintSeed != "" ||
		c.Platform != "" ||
		c.Locale != "" ||
		c.Timezone != "" ||
		c.WebRTCIP != "" ||
		c.FontsDir != "" ||
		c.StorageQuotaMB > 0 ||
		!c.DisableDefaultStealthArgs
	out := CloakBrowserConfig{
		FingerprintSeed: c.FingerprintSeed,
		Platform:        c.Platform,
		Locale:          c.Locale,
		Timezone:        c.Timezone,
		WebRTCIP:        c.WebRTCIP,
		FontsDir:        c.FontsDir,
	}
	if c.StorageQuotaMB > 0 || providerHasNativeStealth {
		out.StorageQuotaMB = intPtrIfNonNegative(c.StorageQuotaMB)
	}
	if hasRuntimeCloak {
		out.DisableDefaultStealthArgs = boolPtrValue(c.DisableDefaultStealthArgs)
	}
	return out
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
		(cfg.TabLifecyclePolicy != "keep" || cfg.TabCloseDelay != 5*time.Minute)
	hasRestore := cfg.TabRestore
	if !hasLifecycle && !hasRestore {
		return nil
	}
	out := &TabPolicyDefaults{}
	if hasLifecycle {
		out.Lifecycle = cfg.TabLifecyclePolicy
		if cfg.TabLifecyclePolicy == "close_idle" && cfg.TabCloseDelay > 0 && cfg.TabCloseDelay != 5*time.Minute {
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

// browsersConfigJSONFromFile copies the browsers block for serialization. The
// retired Config map is still copied for round-trip byte fidelity even though
// validation rejects it — we warn, we don't destroy user input.
func browsersConfigJSONFromFile(bc BrowsersConfig) *BrowsersConfig {
	if bc.Default == "" && len(bc.Available) == 0 && len(bc.Config) == 0 {
		return nil
	}
	out := &BrowsersConfig{
		Default:   bc.Default,
		Available: copyStringSlice(bc.Available),
	}
	if len(bc.Config) > 0 {
		out.Config = make(map[string]BrowserItemConfig, len(bc.Config))
		for k, v := range bc.Config {
			out.Config[k] = v
		}
	}
	return out
}

func (fc FileConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(fileConfigJSON{
		Schema:        fc.Schema,
		ConfigVersion: fc.ConfigVersion,
		Browsers:      browsersConfigJSONFromFile(fc.Browsers),
		Server: serverConfigJSON{
			Port:                      fc.Server.Port,
			Bind:                      fc.Server.Bind,
			Token:                     fc.Server.Token,
			StateDir:                  fc.Server.StateDir,
			NetworkBufferSize:         fc.Server.NetworkBufferSize,
			RetainNetworkBodies:       fc.Server.RetainNetworkBodies,
			RetainNetworkBodyMaxBytes: fc.Server.RetainNetworkBodyMaxBytes,
			TrustProxyHeaders:         fc.Server.TrustProxyHeaders,
			CookieSecure:              fc.Server.CookieSecure,
		},
		Browser: browserConfigJSON{
			Provider:          fc.Browser.Provider, // removed; kept for round-trip fidelity, omitted when empty via omitempty
			BrowserVersion:    fc.Browser.BrowserVersion,
			BrowserBinary:     fc.Browser.BrowserBinary,
			BrowserDebugPort:  fc.Browser.BrowserDebugPort,
			BrowserExtraFlags: fc.Browser.BrowserExtraFlags,
			Cloak:             cloakBrowserConfigJSONFromFile(fc.Browser.Cloak),
			ExtensionPaths:    copyStringSlice(fc.Browser.ExtensionPaths),
			Proxy:             browserProxyJSONFromFile(fc.Browser.Proxy),
			DefaultTarget:     fc.Browser.DefaultTarget,
			FallbackOrder:     fc.Browser.FallbackOrder,
			Targets:           fc.Browser.Targets,
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
			Humanize:          fc.InstanceDefaults.Humanize,
			StealthLevel:      fc.InstanceDefaults.StealthLevel,
			TabEvictionPolicy: fc.InstanceDefaults.TabEvictionPolicy,
			TabPolicy:         fc.InstanceDefaults.TabPolicy,
		},
		Security: securityConfigJSON{
			AllowEvaluate:          fc.Security.AllowEvaluate,
			AllowMacro:             fc.Security.AllowMacro,
			AllowScreencast:        fc.Security.AllowScreencast,
			AllowDownload:          fc.Security.AllowDownload,
			AllowCookies:           fc.Security.AllowCookies,
			AllowNetworkIntercept:  fc.Security.AllowNetworkIntercept,
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
			TrustLoopbackProxy:     fc.Security.TrustLoopbackProxy,
			Attach: attachJSON{
				Enabled:          fc.Security.Attach.Enabled,
				AllowHosts:       copyStringSlice(fc.Security.Attach.AllowHosts),
				AllowSchemes:     copyStringSlice(fc.Security.Attach.AllowSchemes),
				ForwardProxyAuth: fc.Security.Attach.ForwardProxyAuth,
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
	humanize := cfg.Humanize
	allowEvaluate := cfg.AllowEvaluate
	allowMacro := cfg.AllowMacro
	allowScreencast := cfg.AllowScreencast
	allowDownload := cfg.AllowDownload
	allowCookies := cfg.AllowCookies
	allowNetworkIntercept := cfg.AllowNetworkIntercept
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
	trustLoopbackProxy := cfg.TrustLoopbackProxy
	attachEnabled := cfg.AttachEnabled
	attachForwardProxyAuth := cfg.AttachForwardProxyAuth
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
	retainBodies := cfg.RetainNetworkBodies
	retainBodyMaxBytes := cfg.RetainNetworkBodyMaxBytes

	// Always emit browsers.default; stop writing the deprecated browser.provider
	// field. Write the value verbatim (unknown values are kept raw by config load
	// so the load-time warning + chrome fallback still fire — don't normalize and
	// silently rewrite them); only an empty value falls back to chrome.
	browsersDefault := cfg.DefaultBrowser
	if browsersDefault == "" {
		browsersDefault = BrowserChrome
	}
	browsersBlock := BrowsersConfig{
		Default:   browsersDefault,
		Available: append([]string(nil), cfg.BrowsersAvailable...),
	}

	fc := FileConfig{
		Schema: CurrentConfigSchemaURL(),
		Server: ServerConfig{
			Port:                      cfg.Port,
			Bind:                      cfg.Bind,
			Token:                     cfg.Token,
			StateDir:                  cfg.StateDir,
			NetworkBufferSize:         netBufSize,
			RetainNetworkBodies:       &retainBodies,
			RetainNetworkBodyMaxBytes: &retainBodyMaxBytes,
			TrustProxyHeaders:         &cfg.TrustProxyHeaders,
			CookieSecure:              cfg.CookieSecure,
		},
		Browser: BrowserConfig{
			// Provider intentionally omitted — deprecated in favor of browsers.default.
			BrowserVersion:    cfg.BrowserVersion,
			BrowserBinary:     cfg.BrowserBinary,
			BrowserDebugPort:  intPtrIfPositive(cfg.BrowserDebugPort),
			BrowserExtraFlags: cfg.BrowserExtraFlags,
			Cloak:             cloakBrowserConfigFromRuntime(cfg),
			ExtensionPaths:    append([]string(nil), cfg.ExtensionPaths...),
			Proxy:             cloneBrowserProxyConfig(cfg.Proxy),
			DefaultTarget:     cfg.DefaultTarget,
			FallbackOrder:     append([]string(nil), cfg.FallbackOrder...),
			Targets:           cloneBrowserTargetsConfig(cfg.Targets),
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
			Humanize:          &humanize,
			StealthLevel:      cfg.StealthLevel,
			TabEvictionPolicy: cfg.TabEvictionPolicy,
			TabPolicy:         tabPolicyDefaultsFromRuntime(cfg),
		},
		Security: SecurityConfig{
			AllowEvaluate:          &allowEvaluate,
			AllowMacro:             &allowMacro,
			AllowScreencast:        &allowScreencast,
			AllowDownload:          &allowDownload,
			AllowCookies:           &allowCookies,
			AllowNetworkIntercept:  &allowNetworkIntercept,
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
			TrustLoopbackProxy:     &trustLoopbackProxy,
			Attach: AttachConfig{
				Enabled:          &attachEnabled,
				AllowHosts:       append([]string(nil), cfg.AttachAllowHosts...),
				AllowSchemes:     append([]string(nil), cfg.AttachAllowSchemes...),
				ForwardProxyAuth: &attachForwardProxyAuth,
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
		Browsers: browsersBlock,
	}

	reconcileDefaultTargetProvider(&fc.Browser, browsersDefault, cfg)

	return fc
}

// reconcileDefaultTargetProvider keeps the serialized default browser target
// consistent with browsers.default, which is the authoritative provider source.
// config.Load() eagerly synthesizes a "default" target from the legacy chrome
// fields, so when a caller later overrides DefaultBrowser (for example the
// orchestrator selecting cloak for a child instance) without rewriting Targets,
// the stale target would otherwise shadow browsers.default on reload because
// explicit targets win over the legacy fields. Only the lone auto-synthesized
// "default" target is reconciled; user-authored targets (single or multi) are
// left intact — rewriting them would flip the user's provider and wipe
// target-scoped binary/flags/cloak/proxy with global runtime values.
func reconcileDefaultTargetProvider(bc *BrowserConfig, browsersDefault string, cfg *RuntimeConfig) {
	if bc == nil || cfg == nil || !cfg.TargetsSynthesized || len(bc.Targets) != 1 {
		return
	}
	target, ok := bc.Targets[DefaultBrowserTargetName]
	if !ok {
		return
	}
	want := NormalizeBrowser(browsersDefault)
	if NormalizeBrowser(target.Provider) == want {
		return
	}
	// browsers.default won; rewrite the default target from the authoritative
	// runtime fields so the round-trip preserves the selected provider.
	target.Provider = want
	target.Binary = cfg.BrowserBinary
	target.ExtraFlags = cfg.BrowserExtraFlags
	target.Cloak = cloakBrowserConfigFromRuntime(cfg)
	target.Proxy = cloneBrowserProxyConfig(cfg.Proxy)
	bc.Targets[DefaultBrowserTargetName] = target
}
