package config

import (
	"fmt"
	"strconv"
	"strings"
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
	case "sessions":
		return setSessionsField(&fc.Sessions, field, value)
	default:
		return fmt.Errorf("unknown section %q (valid: server, browser, instanceDefaults, security, profiles, multiInstance, timeouts, sessions)", section)
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
	switch field {
	case "version":
		b.ChromeVersion = value
	case "binary":
		b.ChromeBinary = value
	case "extraFlags":
		b.ChromeExtraFlags = value
	default:
		return fmt.Errorf("unknown field browser.%s", field)
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
	case "stealthLevel":
		c.StealthLevel = value
	case "tabEvictionPolicy":
		c.TabEvictionPolicy = value
	default:
		return fmt.Errorf("unknown field instanceDefaults.%s", field)
	}
	return nil
}

func setSecurityField(s *SecurityConfig, field, value string) error {
	if strings.HasPrefix(field, "attach.") {
		return setAttachField(&s.Attach, strings.TrimPrefix(field, "attach."), value)
	}
	if strings.HasPrefix(field, "idpi.") {
		return setIDPIField(&s.IDPI, strings.TrimPrefix(field, "idpi."), value)
	}
	if field == "downloadAllowedDomains" {
		s.DownloadAllowedDomains = parseCSVList(value)
		return nil
	}
	if field == "trustedProxyCIDRs" {
		s.TrustedProxyCIDRs = parseCSVList(value)
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
	case "allowUpload":
		s.AllowUpload = &b
	case "enableActionGuards":
		s.EnableActionGuards = &b
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
	default:
		return fmt.Errorf("unknown field security.attach.%s", field)
	}
	return nil
}

func setIDPIField(i *IDPIConfig, field, value string) error {
	switch field {
	case "enabled":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("security.idpi.enabled: %w", err)
		}
		i.Enabled = b
	case "allowedDomains":
		i.AllowedDomains = parseCSVList(value)
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
