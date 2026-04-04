package config

import (
	"fmt"
	"strconv"
	"strings"
)

// GetConfigValue reads a dotted-path field from FileConfig and returns its string representation.
// Pointer fields that are unset return an empty string. Slice fields are comma-separated.
func GetConfigValue(fc *FileConfig, path string) (string, error) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid path %q (expected section.field, e.g., server.port)", path)
	}

	section, field := parts[0], parts[1]

	switch section {
	case "server":
		return getServerField(&fc.Server, field)
	case "browser":
		return getBrowserField(&fc.Browser, field)
	case "instanceDefaults":
		return getInstanceDefaultsField(&fc.InstanceDefaults, field)
	case "security":
		return getSecurityField(&fc.Security, field)
	case "profiles":
		return getProfilesField(&fc.Profiles, field)
	case "multiInstance":
		return getMultiInstanceField(&fc.MultiInstance, field)
	case "timeouts":
		return getTimeoutsField(&fc.Timeouts, field)
	case "sessions":
		return getSessionsField(&fc.Sessions, field)
	default:
		return "", fmt.Errorf("unknown section %q (valid: server, browser, instanceDefaults, security, profiles, multiInstance, timeouts, sessions)", section)
	}
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
	case "trustProxyHeaders":
		return formatBoolPtr(s.TrustProxyHeaders), nil
	case "cookieSecure":
		return formatBoolPtr(s.CookieSecure), nil
	default:
		return "", fmt.Errorf("unknown field server.%s", field)
	}
}

func getBrowserField(b *BrowserConfig, field string) (string, error) {
	switch field {
	case "version":
		return b.ChromeVersion, nil
	case "binary":
		return b.ChromeBinary, nil
	case "extraFlags":
		return b.ChromeExtraFlags, nil
	default:
		return "", fmt.Errorf("unknown field browser.%s", field)
	}
}

func getSessionsField(s *SessionsFileConfig, field string) (string, error) {
	if strings.HasPrefix(field, "dashboard.") {
		return getDashboardSessionField(&s.Dashboard, strings.TrimPrefix(field, "dashboard."))
	}
	return "", fmt.Errorf("unknown field sessions.%s", field)
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
	case "stealthLevel":
		return c.StealthLevel, nil
	case "tabEvictionPolicy":
		return c.TabEvictionPolicy, nil
	default:
		return "", fmt.Errorf("unknown field instanceDefaults.%s", field)
	}
}

func getSecurityField(s *SecurityConfig, field string) (string, error) {
	if strings.HasPrefix(field, "attach.") {
		return getAttachField(&s.Attach, strings.TrimPrefix(field, "attach."))
	}
	if strings.HasPrefix(field, "idpi.") {
		return getIDPIField(&s.IDPI, strings.TrimPrefix(field, "idpi."))
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
	default:
		return "", fmt.Errorf("unknown field security.attach.%s", field)
	}
}

func getIDPIField(i *IDPIConfig, field string) (string, error) {
	switch field {
	case "enabled":
		return strconv.FormatBool(i.Enabled), nil
	case "allowedDomains":
		return strings.Join(i.AllowedDomains, ","), nil
	case "strictMode":
		return strconv.FormatBool(i.StrictMode), nil
	case "scanContent":
		return strconv.FormatBool(i.ScanContent), nil
	case "wrapContent":
		return strconv.FormatBool(i.WrapContent), nil
	case "customPatterns":
		return strings.Join(i.CustomPatterns, ","), nil
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

func formatIntPtr(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}
