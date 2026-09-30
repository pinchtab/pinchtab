package config

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/browsers"
)

func copyStringSlice(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items...)
}

func ptr[T any](v T) *T {
	return &v
}

func secondsPtr(d time.Duration) *int {
	return ptr(int(d / time.Second))
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

// browserProxyJSONFromFile returns nil only when NOTHING is set, so omitempty drops the
// field. It used to drop the whole block whenever server was unset, which discarded
// credentials, bypass list and geo that `config set` had just reported as saved.
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
		if IdleTabLifecycle(cfg.TabLifecyclePolicy) && cfg.TabCloseDelay > 0 && cfg.TabCloseDelay != 5*time.Minute {
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
			LogLevel:                  fc.Server.LogLevel,
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
			Mode:                   fc.InstanceDefaults.Mode,
			NoRestore:              fc.InstanceDefaults.NoRestore,
			Timezone:               fc.InstanceDefaults.Timezone,
			BlockImages:            fc.InstanceDefaults.BlockImages,
			BlockMedia:             fc.InstanceDefaults.BlockMedia,
			BlockAds:               fc.InstanceDefaults.BlockAds,
			MaxTabs:                fc.InstanceDefaults.MaxTabs,
			MaxParallelTabs:        fc.InstanceDefaults.MaxParallelTabs,
			UserAgent:              fc.InstanceDefaults.UserAgent,
			NoAnimations:           fc.InstanceDefaults.NoAnimations,
			CaptureAllowActivation: fc.InstanceDefaults.CaptureAllowActivation,
			Humanize:               fc.InstanceDefaults.Humanize,
			StealthLevel:           fc.InstanceDefaults.StealthLevel,
			TabEvictionPolicy:      fc.InstanceDefaults.TabEvictionPolicy,
			TabPolicy:              fc.InstanceDefaults.TabPolicy,
			DialogAutoAccept:       fc.InstanceDefaults.DialogAutoAccept,
		},
		Security: fc.Security.wire(),
		Profiles: profilesConfigJSON{
			BaseDir:        fc.Profiles.BaseDir,
			DefaultProfile: fc.Profiles.DefaultProfile,
			QuarantineKeep: fc.Profiles.QuarantineKeep,
		},
		MultiInstance: multiInstanceConfigJSON{
			Strategy:          fc.MultiInstance.Strategy,
			AllocationPolicy:  fc.MultiInstance.AllocationPolicy,
			InstancePortStart: fc.MultiInstance.InstancePortStart,
			InstancePortEnd:   fc.MultiInstance.InstancePortEnd,
			Restart:           fc.MultiInstance.Restart,
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
			MaxBatchSize:      fc.Scheduler.MaxBatchSize,
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
		Sessions:   fc.Sessions,
		AutoSolver: fc.AutoSolver,
	})
}

func (s SecurityConfig) wire() securityConfigJSON {
	return securityConfigJSON{
		AllowEvaluate:          s.AllowEvaluate,
		AllowMacro:             s.AllowMacro,
		AllowScreencast:        s.AllowScreencast,
		AllowDownload:          s.AllowDownload,
		AllowCookies:           s.AllowCookies,
		AllowNetworkIntercept:  s.AllowNetworkIntercept,
		AllowMemory:            s.AllowMemory,
		AllowFileScheme:        s.AllowFileScheme,
		AllowedDomains:         effectiveSecurityAllowedDomains(s),
		DownloadAllowedDomains: copyStringSlice(s.DownloadAllowedDomains),
		DownloadMaxBytes:       s.DownloadMaxBytes,
		MemorySnapshotMaxBytes: s.MemorySnapshotMaxBytes,
		AllowUpload:            s.AllowUpload,
		AllowClipboard:         s.AllowClipboard,
		AllowStateExport:       s.AllowStateExport,
		StateEncryptionKey:     s.StateEncryptionKey,
		UploadMaxRequestBytes:  s.UploadMaxRequestBytes,
		UploadMaxFiles:         s.UploadMaxFiles,
		UploadMaxFileBytes:     s.UploadMaxFileBytes,
		UploadMaxTotalBytes:    s.UploadMaxTotalBytes,
		MaxRedirects:           s.MaxRedirects,
		TrustedProxyCIDRs:      copyStringSlice(s.TrustedProxyCIDRs),
		TrustedResolveCIDRs:    copyStringSlice(s.TrustedResolveCIDRs),
		TrustLoopbackProxy:     s.TrustLoopbackProxy,
		Attach: attachJSON{
			Enabled:          s.Attach.Enabled,
			AllowHosts:       copyStringSlice(s.Attach.AllowHosts),
			AllowSchemes:     copyStringSlice(s.Attach.AllowSchemes),
			ForwardProxyAuth: s.Attach.ForwardProxyAuth,
		},
		IDPI: s.IDPI.wire(),
	}
}

func (i *IDPIConfig) wire() idpiConfigJSON {
	if i == nil {
		i = &IDPIConfig{}
	}
	return idpiConfigJSON{
		Enabled:         i.Enabled,
		StrictMode:      i.StrictMode,
		ScanContent:     i.ScanContent,
		WrapContent:     i.WrapContent,
		CustomPatterns:  copyStringSlice(i.CustomPatterns),
		ScanTimeoutSec:  i.ScanTimeoutSec,
		ShieldThreshold: i.ShieldThreshold,
	}
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
	browsersDefault := cfg.DefaultBrowser
	if browsersDefault == "" {
		browsersDefault = BrowserChrome
	}
	fc := FileConfig{
		Schema:           CurrentConfigSchemaURL(),
		Server:           serverConfigFromRuntime(cfg),
		Browser:          browserConfigFromRuntime(cfg),
		InstanceDefaults: instanceDefaultsFromRuntime(cfg),
		Security:         securityConfigFromRuntime(cfg),
		Profiles:         profilesConfigFromRuntime(cfg),
		MultiInstance:    multiInstanceConfigFromRuntime(cfg),
		Timeouts:         timeoutsConfigFromRuntime(cfg),
		Observability:    observabilityConfigFromRuntime(cfg),
		Sessions:         sessionsConfigFromRuntime(cfg),
		AutoSolver:       autoSolverConfigFromRuntime(cfg),
		Browsers:         BrowsersConfig{Default: browsersDefault, Available: cloneStringSlice(cfg.BrowsersAvailable)},
	}
	reconcileDefaultTargetProvider(&fc.Browser, browsersDefault, cfg)
	return fc
}

func serverConfigFromRuntime(cfg *RuntimeConfig) ServerConfig {
	return ServerConfig{
		Port:                      cfg.Port,
		Bind:                      cfg.Bind,
		Token:                     cfg.Token,
		StateDir:                  cfg.StateDir,
		LogLevel:                  cfg.LogLevel,
		NetworkBufferSize:         intPtrIfPositive(cfg.NetworkBufferSize),
		RetainNetworkBodies:       ptr(cfg.RetainNetworkBodies),
		RetainNetworkBodyMaxBytes: ptr(cfg.RetainNetworkBodyMaxBytes),
		TrustProxyHeaders:         ptr(cfg.TrustProxyHeaders),
		CookieSecure:              cloneBoolPtr(cfg.CookieSecure),
	}
}

func browserConfigFromRuntime(cfg *RuntimeConfig) BrowserConfig {
	return BrowserConfig{
		BrowserVersion:    cfg.BrowserVersion,
		BrowserBinary:     cfg.BrowserBinary,
		BrowserDebugPort:  intPtrIfPositive(cfg.BrowserDebugPort),
		BrowserExtraFlags: cfg.BrowserExtraFlags,
		Cloak:             cloakBrowserConfigFromRuntime(cfg),
		ExtensionPaths:    cloneStringSlice(cfg.ExtensionPaths),
		Proxy:             cloneBrowserProxyConfig(cfg.Proxy),
		DefaultTarget:     cfg.DefaultTarget,
		FallbackOrder:     cloneStringSlice(cfg.FallbackOrder),
		Targets:           cloneBrowserTargetsConfig(cfg.Targets),
	}
}

func instanceDefaultsFromRuntime(cfg *RuntimeConfig) InstanceDefaultsConfig {
	mode := "headless"
	if !cfg.Headless {
		mode = "headed"
	}
	return InstanceDefaultsConfig{
		Mode:                   mode,
		NoRestore:              ptr(cfg.NoRestore),
		Timezone:               cfg.Timezone,
		BlockImages:            ptr(cfg.BlockImages),
		BlockMedia:             ptr(cfg.BlockMedia),
		BlockAds:               ptr(cfg.BlockAds),
		MaxTabs:                ptr(cfg.MaxTabs),
		MaxParallelTabs:        ptr(cfg.MaxParallelTabs),
		UserAgent:              cfg.UserAgent,
		NoAnimations:           ptr(cfg.NoAnimations),
		CaptureAllowActivation: ptr(cfg.CaptureAllowActivation),
		Humanize:               ptr(cfg.Humanize),
		StealthLevel:           cfg.StealthLevel,
		TabEvictionPolicy:      cfg.TabEvictionPolicy,
		TabPolicy:              tabPolicyDefaultsFromRuntime(cfg),
		DialogAutoAccept:       ptr(cfg.DialogAutoAccept),
	}
}

func securityConfigFromRuntime(cfg *RuntimeConfig) SecurityConfig {
	idpi := cfg.IDPI
	idpi.CustomPatterns = cloneStringSlice(cfg.IDPI.CustomPatterns)
	return SecurityConfig{
		AllowEvaluate:          ptr(cfg.AllowEvaluate),
		AllowMacro:             ptr(cfg.AllowMacro),
		AllowScreencast:        ptr(cfg.AllowScreencast),
		AllowDownload:          ptr(cfg.AllowDownload),
		AllowCookies:           ptr(cfg.AllowCookies),
		AllowNetworkIntercept:  ptr(cfg.AllowNetworkIntercept),
		AllowMemory:            ptr(cfg.AllowMemory),
		AllowFileScheme:        ptr(cfg.AllowFileScheme),
		AllowedDomains:         cloneStringSlice(cfg.AllowedDomains),
		DownloadAllowedDomains: cloneStringSlice(cfg.DownloadAllowedDomains),
		DownloadMaxBytes:       ptr(cfg.EffectiveDownloadMaxBytes()),
		MemorySnapshotMaxBytes: ptr(cfg.EffectiveMemorySnapshotMaxBytes()),
		AllowUpload:            ptr(cfg.AllowUpload),
		AllowClipboard:         ptr(cfg.AllowClipboard),
		AllowStateExport:       ptr(cfg.AllowStateExport),
		UploadMaxRequestBytes:  ptr(cfg.EffectiveUploadMaxRequestBytes()),
		UploadMaxFiles:         ptr(cfg.EffectiveUploadMaxFiles()),
		UploadMaxFileBytes:     ptr(cfg.EffectiveUploadMaxFileBytes()),
		UploadMaxTotalBytes:    ptr(cfg.EffectiveUploadMaxTotalBytes()),
		MaxRedirects:           ptr(cfg.MaxRedirects),
		TrustedProxyCIDRs:      cloneStringSlice(cfg.TrustedProxyCIDRs),
		TrustedResolveCIDRs:    cloneStringSlice(cfg.TrustedResolveCIDRs),
		TrustLoopbackProxy:     ptr(cfg.TrustLoopbackProxy),
		Attach: AttachConfig{
			Enabled:          ptr(cfg.AttachEnabled),
			AllowHosts:       cloneStringSlice(cfg.AttachAllowHosts),
			AllowSchemes:     cloneStringSlice(cfg.AttachAllowSchemes),
			ForwardProxyAuth: ptr(cfg.AttachForwardProxyAuth),
		},
		IDPI: &idpi,
	}
}

func profilesConfigFromRuntime(cfg *RuntimeConfig) ProfilesConfig {
	return ProfilesConfig{
		BaseDir:        cfg.ProfilesBaseDir,
		DefaultProfile: cfg.DefaultProfile,
		QuarantineKeep: ptr(cfg.ProfileQuarantineKeep),
	}
}

func multiInstanceConfigFromRuntime(cfg *RuntimeConfig) MultiInstanceConfig {
	return MultiInstanceConfig{
		Strategy:          cfg.Strategy,
		AllocationPolicy:  cfg.AllocationPolicy,
		InstancePortStart: ptr(cfg.InstancePortStart),
		InstancePortEnd:   ptr(cfg.InstancePortEnd),
		Restart: MultiInstanceRestartConfig{
			MaxRestarts:    ptr(cfg.RestartMaxRestarts),
			InitBackoffSec: secondsPtr(cfg.RestartInitBackoff),
			MaxBackoffSec:  secondsPtr(cfg.RestartMaxBackoff),
			StableAfterSec: secondsPtr(cfg.RestartStableAfter),
		},
	}
}

func timeoutsConfigFromRuntime(cfg *RuntimeConfig) TimeoutsConfig {
	return TimeoutsConfig{
		ActionSec:   int(cfg.ActionTimeout / time.Second),
		NavigateSec: int(cfg.NavigateTimeout / time.Second),
		ShutdownSec: int(cfg.ShutdownTimeout / time.Second),
		WaitNavMs:   int(cfg.WaitNavDelay / time.Millisecond),
	}
}

func observabilityConfigFromRuntime(cfg *RuntimeConfig) ObservabilityFileConfig {
	activity := cfg.Observability.Activity
	return ObservabilityFileConfig{
		Activity: ActivityFileConfig{
			Enabled:        ptr(activity.Enabled),
			SessionIdleSec: ptr(activity.SessionIdleSec),
			RetentionDays:  ptr(activity.RetentionDays),
			Events: ActivityEventsFileConfig{
				Dashboard:    ptr(activity.Events.Dashboard),
				Server:       ptr(activity.Events.Server),
				Bridge:       ptr(activity.Events.Bridge),
				Orchestrator: ptr(activity.Events.Orchestrator),
				Scheduler:    ptr(activity.Events.Scheduler),
				MCP:          ptr(activity.Events.MCP),
				Other:        ptr(activity.Events.Other),
			},
		},
	}
}

func sessionsConfigFromRuntime(cfg *RuntimeConfig) SessionsFileConfig {
	dashboard := cfg.Sessions.Dashboard
	agent := cfg.Sessions.Agent
	return SessionsFileConfig{
		Dashboard: DashboardSessionFileConfig{
			Persist:                       ptr(dashboard.Persist),
			IdleTimeoutSec:                secondsPtr(dashboard.IdleTimeout),
			MaxLifetimeSec:                secondsPtr(dashboard.MaxLifetime),
			ElevationWindowSec:            secondsPtr(dashboard.ElevationWindow),
			PersistElevationAcrossRestart: ptr(dashboard.PersistElevationAcrossRestart),
			RequireElevation:              ptr(dashboard.RequireElevation),
		},
		Agent: AgentSessionFileConfig{
			Enabled:        ptr(agent.Enabled),
			Mode:           agent.Mode,
			IdleTimeoutSec: secondsPtr(agent.IdleTimeout),
			MaxLifetimeSec: secondsPtr(agent.MaxLifetime),
		},
	}
}

func autoSolverConfigFromRuntime(cfg *RuntimeConfig) AutoSolverFileConfig {
	a := cfg.AutoSolver
	return AutoSolverFileConfig{
		Enabled:           ptr(a.Enabled),
		AutoTrigger:       ptr(a.AutoTrigger),
		TriggerOnNavigate: ptr(a.TriggerOnNavigate),
		TriggerOnAction:   ptr(a.TriggerOnAction),
		MaxAttempts:       ptr(a.MaxAttempts),
		SolverTimeoutSec:  ptr(a.SolverTimeoutSec),
		RetryBaseDelayMs:  ptr(a.RetryBaseDelayMs),
		RetryMaxDelayMs:   ptr(a.RetryMaxDelayMs),
		Solvers:           cloneStringSlice(a.Solvers),
		LLMProvider:       a.LLMProvider,
		LLMFallback:       ptr(a.LLMFallback),
		External: AutoSolverExtConf{
			CapsolverKey:  a.CapsolverKey,
			TwoCaptchaKey: a.TwoCaptchaKey,
		},
		Credentials: AutoSolverCredentialsConf{
			Login: AutoSolverLoginConf{
				User:     a.Credentials.Login.User,
				Password: a.Credentials.Login.Password,
			},
			Signup: AutoSolverSignupConf{
				Name:     a.Credentials.Signup.Name,
				Email:    a.Credentials.Signup.Email,
				Password: a.Credentials.Signup.Password,
			},
			Form: AutoSolverFormConf{
				Field1: a.Credentials.Form.Field1,
				Field2: a.Credentials.Form.Field2,
				Email:  a.Credentials.Form.Email,
			},
		},
	}
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
