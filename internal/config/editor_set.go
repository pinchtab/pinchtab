package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
)

// SetConfigValue sets a dotted path in FileConfig (e.g., "server.port", "instanceDefaults.mode").
func SetConfigValue(fc *FileConfig, path string, value string) error {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid path %q (expected section.field, e.g., server.port)", path)
	}

	section, field := parts[0], parts[1]

	switch section {
	case "server":
		return setServerField(&fc.Server, field, value)
	case "browser":
		return setBrowserField(&fc.Browser, field, value)
	case "browsers":
		return setBrowsersField(&fc.Browsers, field, value)
	case "instanceDefaults":
		return setInstanceDefaultsField(&fc.InstanceDefaults, field, value)
	case "security":
		return setSecurityField(&fc.Security, field, value)
	case "profiles":
		return setProfilesField(&fc.Profiles, field, value)
	case "multiInstance":
		return setMultiInstanceField(&fc.MultiInstance, field, value)
	case "timeouts":
		return setTimeoutsField(&fc.Timeouts, field, value)
	case "observability":
		return setObservabilityField(&fc.Observability, field, value)
	case "sessions":
		return setSessionsField(&fc.Sessions, field, value)
	default:
		return fmt.Errorf("unknown section %q (valid: server, browser, browsers, instanceDefaults, security, profiles, multiInstance, timeouts, observability, sessions)", section)
	}
}

func setServerField(s *ServerConfig, field, value string) error {
	switch field {
	case "port":
		s.Port = value
	case "bind":
		s.Bind = value
	case "token":
		s.Token = value
	case "stateDir":
		s.StateDir = value
	case "trustProxyHeaders":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("server.trustProxyHeaders must be true or false: %w", err)
		}
		s.TrustProxyHeaders = &b
	case "cookieSecure":
		v := strings.ToLower(strings.TrimSpace(value))
		if v == "" || v == "auto" || v == "null" {
			// Unset to enable auto-detect behavior (tri-state: nil = auto-detect).
			s.CookieSecure = nil
			return nil
		}
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("server.cookieSecure must be true or false (or empty/auto/null to unset): %w", err)
		}
		s.CookieSecure = &b
	default:
		return fmt.Errorf("unknown field server.%s", field)
	}
	return nil
}

func setBrowserField(b *BrowserConfig, field, value string) error {
	if strings.HasPrefix(field, "cloak.") {
		return setCloakBrowserField(&b.Cloak, strings.TrimPrefix(field, "cloak."), value)
	}
	if strings.HasPrefix(field, "proxy.") {
		return setBrowserProxyField(&b.Proxy, strings.TrimPrefix(field, "proxy."), value)
	}
	if strings.HasPrefix(field, "targets.") {
		return setBrowserTargetField(&b.Targets, strings.TrimPrefix(field, "targets."), value)
	}
	switch field {
	case "provider":
		return fmt.Errorf("browser.provider is no longer supported; use browsers.default")
	case "version":
		b.BrowserVersion = value
	case "binary":
		b.BrowserBinary = value
	case "extraFlags":
		b.BrowserExtraFlags = value
	case "defaultTarget":
		b.DefaultTarget = value
	case "fallbackOrder":
		b.FallbackOrder = parseCSVList(value)
	default:
		return fmt.Errorf("unknown field browser.%s", field)
	}
	return nil
}

func setBrowsersField(b *BrowsersConfig, field, value string) error {
	switch field {
	case "default":
		id := strings.ToLower(strings.TrimSpace(value))
		if _, ok := browsers.Get(id); !ok {
			return fmt.Errorf("unknown browser %q (known: %v)", value, browsers.IDs())
		}
		b.Default = id
	case "available":
		b.Available = parseCSVList(value)
	default:
		return fmt.Errorf("unknown field browsers.%s", field)
	}
	return nil
}

func setBrowserProxyField(p *BrowserProxyConfig, field, value string) error {
	if strings.HasPrefix(field, "geo.") {
		if p.Geo == nil {
			p.Geo = &BrowserProxyGeoConfig{}
		}
		switch strings.TrimPrefix(field, "geo.") {
		case "timezone":
			p.Geo.Timezone = value
		case "locale":
			p.Geo.Locale = value
		case "webrtcIP":
			p.Geo.WebRTCIP = value
		case "countryISO":
			p.Geo.CountryISO = value
		default:
			return fmt.Errorf("unknown field proxy.%s", field)
		}
		return nil
	}
	switch field {
	case "server":
		p.Server = value
	case "bypassList":
		p.BypassList = parseCSVList(value)
	case "username":
		p.Username = value
	case "password":
		p.Password = value
	default:
		return fmt.Errorf("unknown field proxy.%s", field)
	}
	return nil
}

func setBrowserTargetField(targets *BrowserTargetsConfig, path, value string) error {
	name, field, ok := strings.Cut(path, ".")
	if !ok || name == "" || field == "" {
		return fmt.Errorf("invalid browser.targets path %q (expected targets.<name>.<field>)", path)
	}
	if !IsValidBrowserTargetName(name) {
		return fmt.Errorf("invalid browser target name %q (must match ^[a-z][a-z0-9-]{0,31}$)", name)
	}
	if *targets == nil {
		*targets = BrowserTargetsConfig{}
	}
	t := (*targets)[name]
	switch {
	case strings.HasPrefix(field, "cloak."):
		if err := setCloakBrowserField(&t.Cloak, strings.TrimPrefix(field, "cloak."), value); err != nil {
			return err
		}
	case strings.HasPrefix(field, "proxy."):
		if err := setBrowserProxyField(&t.Proxy, strings.TrimPrefix(field, "proxy."), value); err != nil {
			return err
		}
	case field == "provider":
		id := strings.ToLower(strings.TrimSpace(value))
		if _, ok := browsers.Get(id); !ok {
			return fmt.Errorf("unknown browser %q (known: %v)", value, browsers.IDs())
		}
		t.Provider = id
	case field == "binary":
		t.Binary = value
	case field == "extraFlags":
		t.ExtraFlags = value
	default:
		return fmt.Errorf("unknown field browser.targets.%s.%s", name, field)
	}
	(*targets)[name] = t
	return nil
}

func setCloakBrowserField(c *CloakBrowserConfig, field, value string) error {
	switch field {
	case "fingerprintSeed":
		c.FingerprintSeed = value
	case "platform":
		c.Platform = value
	case "locale":
		c.Locale = value
	case "timezone":
		c.Timezone = value
	case "webrtcIP":
		c.WebRTCIP = value
	case "fontsDir":
		c.FontsDir = value
	case "storageQuotaMB":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("browser.cloak.storageQuotaMB must be a number: %w", err)
		}
		c.StorageQuotaMB = &n
	case "disableDefaultStealthArgs":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("browser.cloak.disableDefaultStealthArgs must be true or false: %w", err)
		}
		c.DisableDefaultStealthArgs = &b
	default:
		return fmt.Errorf("unknown field browser.cloak.%s", field)
	}
	return nil
}

func setObservabilityField(o *ObservabilityFileConfig, field, value string) error {
	if strings.HasPrefix(field, "activity.") {
		return setActivityField(&o.Activity, strings.TrimPrefix(field, "activity."), value)
	}
	return fmt.Errorf("unknown field observability.%s", field)
}

func setActivityField(a *ActivityFileConfig, field, value string) error {
	if strings.HasPrefix(field, "events.") {
		return setActivityEventField(&a.Events, strings.TrimPrefix(field, "events."), value)
	}

	switch field {
	case "enabled":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("observability.activity.enabled: %w", err)
		}
		a.Enabled = &b
	case "sessionIdleSec":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("observability.activity.sessionIdleSec must be a number: %w", err)
		}
		a.SessionIdleSec = &n
	case "retentionDays":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("observability.activity.retentionDays must be a number: %w", err)
		}
		a.RetentionDays = &n
	case "stateDir":
		a.StateDir = value
	default:
		return fmt.Errorf("unknown field observability.activity.%s", field)
	}
	return nil
}

func setActivityEventField(e *ActivityEventsFileConfig, field, value string) error {
	b, err := parseBool(value)
	if err != nil {
		return fmt.Errorf("observability.activity.events.%s: %w", field, err)
	}

	switch field {
	case "dashboard":
		e.Dashboard = &b
	case "server":
		e.Server = &b
	case "bridge":
		e.Bridge = &b
	case "orchestrator":
		e.Orchestrator = &b
	case "scheduler":
		e.Scheduler = &b
	case "mcp":
		e.MCP = &b
	case "other":
		e.Other = &b
	default:
		return fmt.Errorf("unknown field observability.activity.events.%s", field)
	}
	return nil
}

func setSessionsField(s *SessionsFileConfig, field, value string) error {
	if strings.HasPrefix(field, "dashboard.") {
		return setDashboardSessionField(&s.Dashboard, strings.TrimPrefix(field, "dashboard."), value)
	}
	return fmt.Errorf("unknown field sessions.%s", field)
}

func setDashboardSessionField(s *DashboardSessionFileConfig, field, value string) error {
	switch field {
	case "persist":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("sessions.dashboard.persist: %w", err)
		}
		s.Persist = &b
	case "idleTimeoutSec":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("sessions.dashboard.idleTimeoutSec must be a number: %w", err)
		}
		s.IdleTimeoutSec = &n
	case "maxLifetimeSec":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("sessions.dashboard.maxLifetimeSec must be a number: %w", err)
		}
		s.MaxLifetimeSec = &n
	case "elevationWindowSec":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("sessions.dashboard.elevationWindowSec must be a number: %w", err)
		}
		s.ElevationWindowSec = &n
	case "persistElevationAcrossRestart":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("sessions.dashboard.persistElevationAcrossRestart: %w", err)
		}
		s.PersistElevationAcrossRestart = &b
	case "requireElevation":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("sessions.dashboard.requireElevation: %w", err)
		}
		s.RequireElevation = &b
	default:
		return fmt.Errorf("unknown field sessions.dashboard.%s", field)
	}
	return nil
}

func setInstanceDefaultsField(c *InstanceDefaultsConfig, field, value string) error {
	if strings.HasPrefix(field, "tabPolicy.") {
		if c.TabPolicy == nil {
			c.TabPolicy = &TabPolicyDefaults{}
		}
		return setTabPolicyField(c.TabPolicy, strings.TrimPrefix(field, "tabPolicy."), value)
	}
	switch field {
	case "mode":
		c.Mode = value
	case "noRestore":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.noRestore: %w", err)
		}
		c.NoRestore = &b
	case "timezone":
		c.Timezone = value
	case "blockImages":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.blockImages: %w", err)
		}
		c.BlockImages = &b
	case "blockMedia":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.blockMedia: %w", err)
		}
		c.BlockMedia = &b
	case "blockAds":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.blockAds: %w", err)
		}
		c.BlockAds = &b
	case "maxTabs":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.maxTabs must be a number: %w", err)
		}
		c.MaxTabs = &n
	case "maxParallelTabs":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.maxParallelTabs must be a number: %w", err)
		}
		c.MaxParallelTabs = &n
	case "userAgent":
		c.UserAgent = value
	case "noAnimations":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.noAnimations: %w", err)
		}
		c.NoAnimations = &b
	case "humanize":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.humanize: %w", err)
		}
		c.Humanize = &b
	case "stealthLevel":
		c.StealthLevel = value
	case "tabEvictionPolicy":
		c.TabEvictionPolicy = value
	default:
		return fmt.Errorf("unknown field instanceDefaults.%s", field)
	}
	return nil
}

func setTabPolicyField(tp *TabPolicyDefaults, field, value string) error {
	switch field {
	case "eviction":
		tp.Eviction = value
	case "lifecycle":
		tp.Lifecycle = value
	case "closeDelaySec":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("instanceDefaults.tabPolicy.closeDelaySec must be a number: %w", err)
		}
		tp.CloseDelaySec = &n
	default:
		return fmt.Errorf("unknown field instanceDefaults.tabPolicy.%s", field)
	}
	return nil
}

func setSecurityField(s *SecurityConfig, field, value string) error {
	if strings.HasPrefix(field, "attach.") {
		return setAttachField(&s.Attach, strings.TrimPrefix(field, "attach."), value)
	}
	if strings.HasPrefix(field, "idpi.") {
		return setIDPIField(s, strings.TrimPrefix(field, "idpi."), value)
	}
	if field == "allowedDomains" {
		s.AllowedDomains = parseCSVList(value)
		return nil
	}
	if field == "downloadAllowedDomains" {
		s.DownloadAllowedDomains = parseCSVList(value)
		return nil
	}
	if field == "trustedProxyCIDRs" {
		s.TrustedProxyCIDRs = parseCSVList(value)
		return nil
	}
	if field == "trustedResolveCIDRs" {
		s.TrustedResolveCIDRs = parseCSVList(value)
		return nil
	}
	switch field {
	case "downloadMaxBytes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("security.downloadMaxBytes must be a number: %w", err)
		}
		s.DownloadMaxBytes = &n
		return nil
	case "uploadMaxRequestBytes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("security.uploadMaxRequestBytes must be a number: %w", err)
		}
		s.UploadMaxRequestBytes = &n
		return nil
	case "uploadMaxFiles":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("security.uploadMaxFiles must be a number: %w", err)
		}
		s.UploadMaxFiles = &n
		return nil
	case "uploadMaxFileBytes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("security.uploadMaxFileBytes must be a number: %w", err)
		}
		s.UploadMaxFileBytes = &n
		return nil
	case "uploadMaxTotalBytes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("security.uploadMaxTotalBytes must be a number: %w", err)
		}
		s.UploadMaxTotalBytes = &n
		return nil
	case "maxRedirects":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("security.maxRedirects must be a number: %w", err)
		}
		s.MaxRedirects = &n
		return nil
	}

	b, err := parseBool(value)
	if err != nil {
		return fmt.Errorf("security.%s: %w", field, err)
	}

	switch field {
	case "allowEvaluate":
		s.AllowEvaluate = &b
	case "allowClipboard":
		s.AllowClipboard = &b
	case "allowMacro":
		s.AllowMacro = &b
	case "allowScreencast":
		s.AllowScreencast = &b
	case "allowDownload":
		s.AllowDownload = &b
	case "allowCookies":
		s.AllowCookies = &b
	case "allowUpload":
		s.AllowUpload = &b
	case "allowNetworkIntercept":
		s.AllowNetworkIntercept = &b
	case "allowFileScheme":
		s.AllowFileScheme = &b
	case "enableActionGuards":
		s.EnableActionGuards = &b
	case "trustLoopbackProxy":
		s.TrustLoopbackProxy = &b
	default:
		return fmt.Errorf("unknown field security.%s", field)
	}
	return nil
}

func setProfilesField(p *ProfilesConfig, field, value string) error {
	switch field {
	case "baseDir":
		p.BaseDir = value
	case "defaultProfile":
		p.DefaultProfile = value
	default:
		return fmt.Errorf("unknown field profiles.%s", field)
	}
	return nil
}

func setMultiInstanceField(o *MultiInstanceConfig, field, value string) error {
	if strings.HasPrefix(field, "restart.") {
		return setMultiInstanceRestartField(&o.Restart, strings.TrimPrefix(field, "restart."), value)
	}

	switch field {
	case "strategy":
		o.Strategy = value
	case "allocationPolicy":
		o.AllocationPolicy = value
	case "instancePortStart":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("multiInstance.instancePortStart must be a number: %w", err)
		}
		o.InstancePortStart = &n
	case "instancePortEnd":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("multiInstance.instancePortEnd must be a number: %w", err)
		}
		o.InstancePortEnd = &n
	default:
		return fmt.Errorf("unknown field multiInstance.%s", field)
	}
	return nil
}

func setMultiInstanceRestartField(r *MultiInstanceRestartConfig, field, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("multiInstance.restart.%s must be a number: %w", field, err)
	}

	switch field {
	case "maxRestarts":
		r.MaxRestarts = &n
	case "initBackoffSec":
		r.InitBackoffSec = &n
	case "maxBackoffSec":
		r.MaxBackoffSec = &n
	case "stableAfterSec":
		r.StableAfterSec = &n
	default:
		return fmt.Errorf("unknown field multiInstance.restart.%s", field)
	}
	return nil
}

func setTimeoutsField(t *TimeoutsConfig, field, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("timeouts.%s must be a number: %w", field, err)
	}

	switch field {
	case "actionSec":
		t.ActionSec = n
	case "navigateSec":
		t.NavigateSec = n
	case "shutdownSec":
		t.ShutdownSec = n
	case "waitNavMs":
		t.WaitNavMs = n
	default:
		return fmt.Errorf("unknown field timeouts.%s", field)
	}
	return nil
}

func setAttachField(a *AttachConfig, field, value string) error {
	switch field {
	case "enabled":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("security.attach.enabled: %w", err)
		}
		a.Enabled = &b
	case "allowHosts":
		a.AllowHosts = parseCSVList(value)
	case "allowSchemes":
		a.AllowSchemes = parseCSVList(value)
	case "forwardProxyAuth":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("security.attach.forwardProxyAuth: %w", err)
		}
		a.ForwardProxyAuth = &b
	default:
		return fmt.Errorf("unknown field security.attach.%s", field)
	}
	return nil
}

func setIDPIField(s *SecurityConfig, field, value string) error {
	i := &s.IDPI
	switch field {
	case "enabled":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("security.idpi.enabled: %w", err)
		}
		i.Enabled = b
	case "strictMode":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("security.idpi.strictMode: %w", err)
		}
		i.StrictMode = b
	case "scanContent":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("security.idpi.scanContent: %w", err)
		}
		i.ScanContent = b
	case "wrapContent":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("security.idpi.wrapContent: %w", err)
		}
		i.WrapContent = b
	case "customPatterns":
		i.CustomPatterns = parseCSVList(value)
	default:
		return fmt.Errorf("unknown field security.idpi.%s", field)
	}
	return nil
}
