package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/safelog"
)

// GetConfigValue reads a dotted-path field from FileConfig and returns its string representation.
// Pointer fields that are unset return an empty string. Slice fields are comma-separated.
// The section vocabulary lives once, in configSections.
func GetConfigValue(fc *FileConfig, path string) (string, error) {
	section, field, err := lookupConfigSection(path)
	if err != nil {
		return "", err
	}
	return section.get(fc, field)
}

func getServerField(s *ServerConfig, field string) (string, error) {
	switch field {
	case "port":
		return s.Port, nil
	case "bind":
		return s.Bind, nil
	case "token":
		return s.Token, nil
	case "stateDir":
		return s.StateDir, nil
	case "logLevel":
		return s.LogLevel, nil
	case "networkBufferSize":
		return formatIntPtr(s.NetworkBufferSize), nil
	case "retainNetworkBodies":
		return formatBoolPtr(s.RetainNetworkBodies), nil
	case "retainNetworkBodyMaxBytes":
		return formatIntPtr(s.RetainNetworkBodyMaxBytes), nil
	case "trustProxyHeaders":
		return formatBoolPtr(s.TrustProxyHeaders), nil
	case "cookieSecure":
		return formatBoolPtr(s.CookieSecure), nil
	default:
		return "", fmt.Errorf("unknown field server.%s", field)
	}
}

func getBrowserField(b *BrowserConfig, field string) (string, error) {
	if strings.HasPrefix(field, "cloak.") {
		return getCloakBrowserField(&b.Cloak, strings.TrimPrefix(field, "cloak."))
	}
	if strings.HasPrefix(field, "proxy.") {
		return getBrowserProxyField(&b.Proxy, strings.TrimPrefix(field, "proxy."))
	}
	if strings.HasPrefix(field, "targets.") {
		return getBrowserTargetField(b.Targets, strings.TrimPrefix(field, "targets."))
	}
	switch field {
	case "provider":
		return "", fmt.Errorf("browser.provider is no longer supported; use browsers.default")
	case "version":
		return b.BrowserVersion, nil
	case "binary":
		return b.BrowserBinary, nil
	case "extraFlags":
		return b.BrowserExtraFlags, nil
	case "remoteDebuggingPort":
		return formatIntPtr(b.BrowserDebugPort), nil
	case "extensionPaths":
		return strings.Join(b.ExtensionPaths, ","), nil
	case "defaultTarget":
		return b.DefaultTarget, nil
	case "fallbackOrder":
		return strings.Join(b.FallbackOrder, ","), nil
	default:
		return "", fmt.Errorf("unknown field browser.%s", field)
	}
}

// getBrowserProxyField returns proxy values plainly — like server.token, the
// CLI editor reads the operator's own file.
func getBrowserProxyField(p *BrowserProxyConfig, field string) (string, error) {
	if strings.HasPrefix(field, "geo.") {
		if p.Geo == nil {
			return "", nil
		}
		switch strings.TrimPrefix(field, "geo.") {
		case "timezone":
			return p.Geo.Timezone, nil
		case "locale":
			return p.Geo.Locale, nil
		case "webrtcIP":
			return p.Geo.WebRTCIP, nil
		case "countryISO":
			return p.Geo.CountryISO, nil
		default:
			return "", fmt.Errorf("unknown field proxy.%s", field)
		}
	}
	switch field {
	case "server":
		return p.Server, nil
	case "bypassList":
		return strings.Join(p.BypassList, ","), nil
	case "username":
		return p.Username, nil
	case "password":
		return p.Password, nil
	default:
		return "", fmt.Errorf("unknown field proxy.%s", field)
	}
}

func getBrowserTargetField(targets BrowserTargetsConfig, path string) (string, error) {
	name, field, ok := strings.Cut(path, ".")
	if !ok || name == "" || field == "" {
		return "", fmt.Errorf("invalid browser.targets path %q (expected targets.<name>.<field>)", path)
	}
	t, ok := targets[name]
	if !ok {
		return "", fmt.Errorf("browser target %q not found", name)
	}
	switch {
	case strings.HasPrefix(field, "cloak."):
		return getCloakBrowserField(&t.Cloak, strings.TrimPrefix(field, "cloak."))
	case strings.HasPrefix(field, "proxy."):
		return getBrowserProxyField(&t.Proxy, strings.TrimPrefix(field, "proxy."))
	case field == "provider":
		return t.Provider, nil
	case field == "binary":
		return t.Binary, nil
	case field == "extraFlags":
		return t.ExtraFlags, nil
	default:
		return "", fmt.Errorf("unknown field browser.targets.%s.%s", name, field)
	}
}

func getBrowsersField(b *BrowsersConfig, field string) (string, error) {
	switch field {
	case "default":
		return b.Default, nil
	case "available":
		return strings.Join(b.Available, ","), nil
	default:
		return "", fmt.Errorf("unknown field browsers.%s", field)
	}
}

func getCloakBrowserField(c *CloakBrowserConfig, field string) (string, error) {
	switch field {
	case "fingerprintSeed":
		return c.FingerprintSeed, nil
	case "platform":
		return c.Platform, nil
	case "locale":
		return c.Locale, nil
	case "timezone":
		return c.Timezone, nil
	case "webrtcIP":
		return c.WebRTCIP, nil
	case "fontsDir":
		return c.FontsDir, nil
	case "storageQuotaMB":
		return formatIntPtr(c.StorageQuotaMB), nil
	case "disableDefaultStealthArgs":
		return formatBoolPtr(c.DisableDefaultStealthArgs), nil
	default:
		return "", fmt.Errorf("unknown field browser.cloak.%s", field)
	}
}

func getObservabilityField(o *ObservabilityFileConfig, field string) (string, error) {
	if strings.HasPrefix(field, "activity.") {
		return getActivityField(&o.Activity, strings.TrimPrefix(field, "activity."))
	}
	return "", fmt.Errorf("unknown field observability.%s", field)
}

func getActivityField(a *ActivityFileConfig, field string) (string, error) {
	if strings.HasPrefix(field, "events.") {
		return getActivityEventField(&a.Events, strings.TrimPrefix(field, "events."))
	}

	switch field {
	case "enabled":
		return formatBoolPtr(a.Enabled), nil
	case "sessionIdleSec":
		return formatIntPtr(a.SessionIdleSec), nil
	case "retentionDays":
		return formatIntPtr(a.RetentionDays), nil
	case "stateDir":
		return a.StateDir, nil
	default:
		return "", fmt.Errorf("unknown field observability.activity.%s", field)
	}
}

func getActivityEventField(e *ActivityEventsFileConfig, field string) (string, error) {
	switch field {
	case "dashboard":
		return formatBoolPtr(e.Dashboard), nil
	case "server":
		return formatBoolPtr(e.Server), nil
	case "bridge":
		return formatBoolPtr(e.Bridge), nil
	case "orchestrator":
		return formatBoolPtr(e.Orchestrator), nil
	case "scheduler":
		return formatBoolPtr(e.Scheduler), nil
	case "mcp":
		return formatBoolPtr(e.MCP), nil
	case "other":
		return formatBoolPtr(e.Other), nil
	default:
		return "", fmt.Errorf("unknown field observability.activity.events.%s", field)
	}
}

func getSessionsField(s *SessionsFileConfig, field string) (string, error) {
	if strings.HasPrefix(field, "dashboard.") {
		return getDashboardSessionField(&s.Dashboard, strings.TrimPrefix(field, "dashboard."))
	}
	if strings.HasPrefix(field, "agent.") {
		return getAgentSessionField(&s.Agent, strings.TrimPrefix(field, "agent."))
	}
	return "", fmt.Errorf("unknown field sessions.%s", field)
}

// getAgentSessionField addresses the switch behind the whole agent-session flow. It was
// unreachable here while the schema declared it and the loader honoured it, so the only
// way to turn agent sessions on was to hand-edit JSON — and the editor answered "unknown
// field", which claims the key is wrong rather than that it is not settable here.
//
// The cases mirror AgentSessionFileConfig's declared fields, and
// TestTheSessionsEditorAddressesEveryDeclaredField walks that struct rather than this
// list: a hand-listed switch is how a field goes missing from a section that otherwise
// looks complete.
func getAgentSessionField(s *AgentSessionFileConfig, field string) (string, error) {
	switch field {
	case "enabled":
		return formatBoolPtr(s.Enabled), nil
	case "mode":
		return s.Mode, nil
	case "idleTimeoutSec":
		return formatIntPtr(s.IdleTimeoutSec), nil
	case "maxLifetimeSec":
		return formatIntPtr(s.MaxLifetimeSec), nil
	default:
		return "", fmt.Errorf("unknown field sessions.agent.%s", field)
	}
}

func getDashboardSessionField(s *DashboardSessionFileConfig, field string) (string, error) {
	switch field {
	case "persist":
		return formatBoolPtr(s.Persist), nil
	case "idleTimeoutSec":
		return formatIntPtr(s.IdleTimeoutSec), nil
	case "maxLifetimeSec":
		return formatIntPtr(s.MaxLifetimeSec), nil
	case "elevationWindowSec":
		return formatIntPtr(s.ElevationWindowSec), nil
	case "persistElevationAcrossRestart":
		return formatBoolPtr(s.PersistElevationAcrossRestart), nil
	case "requireElevation":
		return formatBoolPtr(s.RequireElevation), nil
	default:
		return "", fmt.Errorf("unknown field sessions.dashboard.%s", field)
	}
}

func getInstanceDefaultsField(c *InstanceDefaultsConfig, field string) (string, error) {
	if after, ok := strings.CutPrefix(field, "tabPolicy."); ok {
		return getTabPolicyField(c.TabPolicy, after)
	}
	switch field {
	case "mode":
		return c.Mode, nil
	case "noRestore":
		return formatBoolPtr(c.NoRestore), nil
	case "timezone":
		return c.Timezone, nil
	case "blockImages":
		return formatBoolPtr(c.BlockImages), nil
	case "blockMedia":
		return formatBoolPtr(c.BlockMedia), nil
	case "blockAds":
		return formatBoolPtr(c.BlockAds), nil
	case "maxTabs":
		return formatIntPtr(c.MaxTabs), nil
	case "maxParallelTabs":
		return formatIntPtr(c.MaxParallelTabs), nil
	case "userAgent":
		return c.UserAgent, nil
	case "noAnimations":
		return formatBoolPtr(c.NoAnimations), nil
	case "captureAllowActivation":
		return formatBoolPtr(c.CaptureAllowActivation), nil
	case "humanize":
		return formatBoolPtr(c.Humanize), nil
	case "stealthLevel":
		return c.StealthLevel, nil
	case "tabEvictionPolicy":
		return c.TabEvictionPolicy, nil
	case "dialogAutoAccept":
		return formatBoolPtr(c.DialogAutoAccept), nil
	default:
		return "", fmt.Errorf("unknown field instanceDefaults.%s", field)
	}
}

func getTabPolicyField(tp *TabPolicyDefaults, field string) (string, error) {
	if tp == nil {
		tp = &TabPolicyDefaults{}
	}
	switch field {
	case "eviction":
		return tp.Eviction, nil
	case "lifecycle":
		return tp.Lifecycle, nil
	case "closeDelaySec":
		return formatIntPtr(tp.CloseDelaySec), nil
	case "restore":
		return formatBoolPtr(tp.Restore), nil
	default:
		return "", fmt.Errorf("unknown field instanceDefaults.tabPolicy.%s", field)
	}
}

func getSecurityField(s *SecurityConfig, field string) (string, error) {
	if strings.HasPrefix(field, "attach.") {
		return getAttachField(&s.Attach, strings.TrimPrefix(field, "attach."))
	}
	if strings.HasPrefix(field, "idpi.") {
		return getIDPIField(s.IDPI, strings.TrimPrefix(field, "idpi."))
	}

	switch field {
	case "allowEvaluate":
		return formatBoolPtr(s.AllowEvaluate), nil
	case "allowClipboard":
		return formatBoolPtr(s.AllowClipboard), nil
	case "allowMacro":
		return formatBoolPtr(s.AllowMacro), nil
	case "allowScreencast":
		return formatBoolPtr(s.AllowScreencast), nil
	case "allowDownload":
		return formatBoolPtr(s.AllowDownload), nil
	case "allowCookies":
		return formatBoolPtr(s.AllowCookies), nil
	case "allowStateExport":
		return formatBoolPtr(s.AllowStateExport), nil
	case "stateEncryptionKey":
		return formatStringPtr(s.StateEncryptionKey), nil
	case "allowNetworkIntercept":
		return formatBoolPtr(s.AllowNetworkIntercept), nil
	case "allowFileScheme":
		return formatBoolPtr(s.AllowFileScheme), nil
	case "allowedDomains":
		return strings.Join(s.AllowedDomains, ","), nil
	case "downloadAllowedDomains":
		return strings.Join(s.DownloadAllowedDomains, ","), nil
	case "downloadMaxBytes":
		return formatIntPtr(s.DownloadMaxBytes), nil
	case "allowUpload":
		return formatBoolPtr(s.AllowUpload), nil
	case "enableActionGuards":
		return formatBoolPtr(s.EnableActionGuards), nil
	case "uploadMaxRequestBytes":
		return formatIntPtr(s.UploadMaxRequestBytes), nil
	case "uploadMaxFiles":
		return formatIntPtr(s.UploadMaxFiles), nil
	case "uploadMaxFileBytes":
		return formatIntPtr(s.UploadMaxFileBytes), nil
	case "uploadMaxTotalBytes":
		return formatIntPtr(s.UploadMaxTotalBytes), nil
	case "maxRedirects":
		return formatIntPtr(s.MaxRedirects), nil
	case "trustedProxyCIDRs":
		return strings.Join(s.TrustedProxyCIDRs, ","), nil
	case "trustedResolveCIDRs":
		return strings.Join(s.TrustedResolveCIDRs, ","), nil
	case "trustLoopbackProxy":
		return formatBoolPtr(s.TrustLoopbackProxy), nil
	default:
		return "", fmt.Errorf("unknown field security.%s", field)
	}
}

func getProfilesField(p *ProfilesConfig, field string) (string, error) {
	switch field {
	case "baseDir":
		return p.BaseDir, nil
	case "defaultProfile":
		return p.DefaultProfile, nil
	case "quarantineKeep":
		if p.QuarantineKeep == nil {
			return "", nil
		}
		return strconv.Itoa(*p.QuarantineKeep), nil
	default:
		return "", fmt.Errorf("unknown field profiles.%s", field)
	}
}

func getMultiInstanceField(o *MultiInstanceConfig, field string) (string, error) {
	if strings.HasPrefix(field, "restart.") {
		return getMultiInstanceRestartField(&o.Restart, strings.TrimPrefix(field, "restart."))
	}

	switch field {
	case "strategy":
		return o.Strategy, nil
	case "allocationPolicy":
		return o.AllocationPolicy, nil
	case "instancePortStart":
		return formatIntPtr(o.InstancePortStart), nil
	case "instancePortEnd":
		return formatIntPtr(o.InstancePortEnd), nil
	default:
		return "", fmt.Errorf("unknown field multiInstance.%s", field)
	}
}

func getMultiInstanceRestartField(r *MultiInstanceRestartConfig, field string) (string, error) {
	switch field {
	case "maxRestarts":
		return formatIntPtr(r.MaxRestarts), nil
	case "initBackoffSec":
		return formatIntPtr(r.InitBackoffSec), nil
	case "maxBackoffSec":
		return formatIntPtr(r.MaxBackoffSec), nil
	case "stableAfterSec":
		return formatIntPtr(r.StableAfterSec), nil
	default:
		return "", fmt.Errorf("unknown field multiInstance.restart.%s", field)
	}
}

func getAttachField(a *AttachConfig, field string) (string, error) {
	switch field {
	case "enabled":
		return formatBoolPtr(a.Enabled), nil
	case "allowHosts":
		return strings.Join(a.AllowHosts, ","), nil
	case "allowSchemes":
		return strings.Join(a.AllowSchemes, ","), nil
	case "forwardProxyAuth":
		return formatBoolPtr(a.ForwardProxyAuth), nil
	default:
		return "", fmt.Errorf("unknown field security.attach.%s", field)
	}
}

func getIDPIField(i *IDPIConfig, field string) (string, error) {
	if i == nil {
		i = &IDPIConfig{}
	}
	switch field {
	case "enabled":
		return strconv.FormatBool(i.Enabled), nil
	case "strictMode":
		return strconv.FormatBool(i.StrictMode), nil
	case "scanContent":
		return strconv.FormatBool(i.ScanContent), nil
	case "wrapContent":
		return strconv.FormatBool(i.WrapContent), nil
	case "customPatterns":
		return strings.Join(i.CustomPatterns, ","), nil
	case "scanTimeoutSec":
		return strconv.Itoa(i.ScanTimeoutSec), nil
	case "shieldThreshold":
		return strconv.Itoa(i.ShieldThreshold), nil
	default:
		return "", fmt.Errorf("unknown field security.idpi.%s", field)
	}
}

func getTimeoutsField(t *TimeoutsConfig, field string) (string, error) {
	switch field {
	case "actionSec":
		return strconv.Itoa(t.ActionSec), nil
	case "navigateSec":
		return strconv.Itoa(t.NavigateSec), nil
	case "shutdownSec":
		return strconv.Itoa(t.ShutdownSec), nil
	case "waitNavMs":
		return strconv.Itoa(t.WaitNavMs), nil
	default:
		return "", fmt.Errorf("unknown field timeouts.%s", field)
	}
}

func formatBoolPtr(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "true"
	}
	return "false"
}

func formatStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatIntPtr(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func getSchedulerField(s *SchedulerFileConfig, field string) (string, error) {
	switch field {
	case "enabled":
		return formatBoolPtr(s.Enabled), nil
	case "strategy":
		return s.Strategy, nil
	case "maxQueueSize":
		return formatIntPtr(s.MaxQueueSize), nil
	case "maxPerAgent":
		return formatIntPtr(s.MaxPerAgent), nil
	case "maxInflight":
		return formatIntPtr(s.MaxInflight), nil
	case "maxPerAgentInflight":
		return formatIntPtr(s.MaxPerAgentFlight), nil
	case "resultTTLSec":
		return formatIntPtr(s.ResultTTLSec), nil
	case "workerCount":
		return formatIntPtr(s.WorkerCount), nil
	case "maxBatchSize":
		return formatIntPtr(s.MaxBatchSize), nil
	default:
		return "", fmt.Errorf("unknown field scheduler.%s", field)
	}
}

func getAutoSolverField(a *AutoSolverFileConfig, field string) (string, error) {
	if rest, ok := strings.CutPrefix(field, "external."); ok {
		return getAutoSolverExternalField(&a.External, rest)
	}
	if rest, ok := strings.CutPrefix(field, "credentials."); ok {
		return getAutoSolverCredentialsField(&a.Credentials, rest)
	}

	switch field {
	case "enabled":
		return formatBoolPtr(a.Enabled), nil
	case "autoTrigger":
		return formatBoolPtr(a.AutoTrigger), nil
	case "triggerOnNavigate":
		return formatBoolPtr(a.TriggerOnNavigate), nil
	case "triggerOnAction":
		return formatBoolPtr(a.TriggerOnAction), nil
	case "llmFallback":
		return formatBoolPtr(a.LLMFallback), nil
	case "maxAttempts":
		return formatIntPtr(a.MaxAttempts), nil
	case "solverTimeoutSec":
		return formatIntPtr(a.SolverTimeoutSec), nil
	case "retryBaseDelayMs":
		return formatIntPtr(a.RetryBaseDelayMs), nil
	case "retryMaxDelayMs":
		return formatIntPtr(a.RetryMaxDelayMs), nil
	case "llmProvider":
		return a.LLMProvider, nil
	case "solvers":
		return strings.Join(a.Solvers, ","), nil
	default:
		return "", fmt.Errorf("unknown field autoSolver.%s", field)
	}
}

func getAutoSolverExternalField(e *AutoSolverExtConf, field string) (string, error) {
	switch field {
	case "capsolverKey":
		return e.CapsolverKey, nil
	case "twoCaptchaKey":
		return e.TwoCaptchaKey, nil
	default:
		return "", fmt.Errorf("unknown field autoSolver.external.%s", field)
	}
}

func getAutoSolverCredentialsField(c *AutoSolverCredentialsConf, field string) (string, error) {
	switch {
	case strings.HasPrefix(field, "login."):
		switch strings.TrimPrefix(field, "login.") {
		case "user":
			return c.Login.User, nil
		case "password":
			return c.Login.Password, nil
		}
	case strings.HasPrefix(field, "signup."):
		switch strings.TrimPrefix(field, "signup.") {
		case "name":
			return c.Signup.Name, nil
		case "email":
			return c.Signup.Email, nil
		case "password":
			return c.Signup.Password, nil
		}
	case strings.HasPrefix(field, "form."):
		switch strings.TrimPrefix(field, "form.") {
		case "field1":
			return c.Form.Field1, nil
		case "field2":
			return c.Form.Field2, nil
		case "email":
			return c.Form.Email, nil
		}
	}
	return "", fmt.Errorf("unknown field autoSolver.credentials.%s", field)
}

// EffectiveConfigValue answers with the value in effect for a dotted path: the file
// value when the file sets one, otherwise the value the runtime resolves. Keys whose
// effective value is derived from another key, or whose default is applied by a
// consumer rather than stored in the config struct, are only visible this way.
func EffectiveConfigValue(path string) (string, error) {
	fc, _, err := LoadFileConfig()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	value, err := GetConfigValue(fc, path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) != "" && !runtimeSettlesTheValue(path) {
		return value, nil
	}
	return resolvedConfigValue(path)
}

// runtimeSettlesTheValue marks the paths whose file entry is an input rather than the
// answer, so reading the file would report something nothing acts on:
//
//	scheduler.*                      a configured zero means "use the default" here
//	observability.activity.stateDir  the path derives from server.stateDir; a file value
//	                                 is refused at config set and warned about at load
func runtimeSettlesTheValue(path string) bool {
	return strings.HasPrefix(path, "scheduler.") || path == "observability.activity.stateDir"
}

func resolvedConfigValue(path string) (string, error) {
	cfg, _, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	resolved := FileConfigFromRuntime(cfg)
	addSettledDefaults(&resolved, cfg)
	return GetConfigValue(&resolved, path)
}

// addSettledDefaults fills the sections FileConfigFromRuntime leaves out on purpose:
// it serialises configs for writing, where emitting every default would add noise to
// a file the operator did not ask for. The reader needs the opposite, so it takes the
// values back off the RuntimeConfig that has already settled them — never from a
// literal spelled here, which would be a second copy free to drift.
func addSettledDefaults(resolved *FileConfig, cfg *RuntimeConfig) {
	if strings.TrimSpace(resolved.Server.LogLevel) == "" {
		resolved.Server.LogLevel = safelog.LevelName(safelog.DefaultLevel)
	}
	// The writer omits a false tab-restore (a vanilla tab policy is left out of a file the
	// operator did not ask for) and omits the state encryption key entirely, so a generated
	// child instance config never carries a secret. Both are settled values a reader has to
	// see, taken off the RuntimeConfig rather than restated here.
	if resolved.InstanceDefaults.TabPolicy == nil {
		resolved.InstanceDefaults.TabPolicy = &TabPolicyDefaults{}
	}
	if resolved.InstanceDefaults.TabPolicy.Restore == nil {
		resolved.InstanceDefaults.TabPolicy.Restore = boolPtrValue(cfg.TabRestore)
	}
	if resolved.Security.StateEncryptionKey == nil && cfg.StateEncryptionKey != "" {
		key := cfg.StateEncryptionKey
		resolved.Security.StateEncryptionKey = &key
	}
	resolved.Scheduler = SchedulerFileConfig{
		Enabled:           boolPtrValue(cfg.Scheduler.Enabled),
		Strategy:          cfg.Scheduler.Strategy,
		MaxQueueSize:      intPtrIfPositive(cfg.Scheduler.MaxQueueSize),
		MaxPerAgent:       intPtrIfPositive(cfg.Scheduler.MaxPerAgent),
		MaxInflight:       intPtrIfPositive(cfg.Scheduler.MaxInflight),
		MaxPerAgentFlight: intPtrIfPositive(cfg.Scheduler.MaxPerAgentFlight),
		ResultTTLSec:      intPtrIfPositive(cfg.Scheduler.ResultTTLSec),
		WorkerCount:       intPtrIfPositive(cfg.Scheduler.WorkerCount),
		MaxBatchSize:      intPtrIfPositive(cfg.Scheduler.MaxBatchSize),
	}
	if resolved.InstanceDefaults.TabPolicy == nil {
		resolved.InstanceDefaults.TabPolicy = &TabPolicyDefaults{}
	}
	tabPolicy := resolved.InstanceDefaults.TabPolicy
	if tabPolicy.Eviction == "" {
		tabPolicy.Eviction = cfg.TabEvictionPolicy
	}
	if tabPolicy.Lifecycle == "" {
		tabPolicy.Lifecycle = cfg.TabLifecyclePolicy
	}
	if tabPolicy.CloseDelaySec == nil {
		tabPolicy.CloseDelaySec = intPtrIfPositive(int(cfg.TabCloseDelay / time.Second))
	}
	if strings.TrimSpace(resolved.Observability.Activity.StateDir) == "" {
		resolved.Observability.Activity.StateDir = cfg.ActivityLogDir()
	}
}
