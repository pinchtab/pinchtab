package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SetConfigValue sets a dotted path in FileConfig (e.g., "server.port", "instanceDefaults.mode").
// Returns the updated FileConfig and any error.
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
	default:
		return fmt.Errorf("unknown section %q (valid: server, browser, instanceDefaults, security, profiles, multiInstance, timeouts)", section)
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

	b, err := parseBool(value)
	if err != nil {
		return fmt.Errorf("security.%s: %w", field, err)
	}

	switch field {
	case "allowEvaluate":
		s.AllowEvaluate = &b
	case "allowMacro":
		s.AllowMacro = &b
	case "allowScreencast":
		s.AllowScreencast = &b
	case "allowDownload":
		s.AllowDownload = &b
	case "allowUpload":
		s.AllowUpload = &b
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
	case "disableLocalLaunch":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("multiInstance.disableLocalLaunch must be true or false: %w", err)
		}
		o.DisableLocalLaunch = &b
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

// GetConfigValue reads a dotted-path field from FileConfig and returns its string representation.
// The path format matches SetConfigValue (e.g., "server.port", "security.attach.allowHosts").
// Pointer fields that are unset return an empty string.
// Slice fields are returned as comma-separated values.
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
	default:
		return "", fmt.Errorf("unknown section %q (valid: server, browser, instanceDefaults, security, profiles, multiInstance, timeouts)", section)
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
	case "allowMacro":
		return formatBoolPtr(s.AllowMacro), nil
	case "allowScreencast":
		return formatBoolPtr(s.AllowScreencast), nil
	case "allowDownload":
		return formatBoolPtr(s.AllowDownload), nil
	case "allowUpload":
		return formatBoolPtr(s.AllowUpload), nil
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
	case "disableLocalLaunch":
		return formatBoolPtr(o.DisableLocalLaunch), nil
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

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q (use true/false)", s)
	}
}

func parseCSVList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// PatchConfigJSON merges a JSON object into FileConfig.
// The patch should be a valid JSON object with the same structure as FileConfig.
func PatchConfigJSON(fc *FileConfig, jsonPatch string) error {
	// First, serialize current config to JSON
	current, err := json.Marshal(fc)
	if err != nil {
		return fmt.Errorf("failed to serialize current config: %w", err)
	}

	// Unmarshal current into a map for merging
	var currentMap map[string]interface{}
	if err := json.Unmarshal(current, &currentMap); err != nil {
		return fmt.Errorf("failed to parse current config: %w", err)
	}

	// Parse the patch
	var patchMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPatch), &patchMap); err != nil {
		return fmt.Errorf("invalid JSON patch: %w", err)
	}

	// Deep merge patch into current
	merged := deepMerge(currentMap, patchMap)

	// Serialize merged back to JSON
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("failed to serialize merged config: %w", err)
	}

	// Unmarshal back into FileConfig
	if err := json.Unmarshal(mergedJSON, fc); err != nil {
		return fmt.Errorf("failed to parse merged config: %w", err)
	}

	return nil
}

// deepMerge recursively merges src into dst, returning the result.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy dst
	for k, v := range dst {
		result[k] = v
	}

	// Merge src
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := result[k].(map[string]interface{}); ok {
				result[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}

// LoadFileConfig loads a FileConfig from the default or specified path.
// Returns the config and the path it was loaded from.
func LoadFileConfig() (*FileConfig, string, error) {
	configPath := envOr("PINCHTAB_CONFIG", filepath.Join(userConfigDir(), "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return &FileConfig{}, configPath, nil
		}
		return nil, configPath, fmt.Errorf("failed to read config file: %w", err)
	}

	var fc *FileConfig

	if isLegacyConfig(data) {
		var lc legacyFileConfig
		if err := json.Unmarshal(data, &lc); err != nil {
			return nil, configPath, fmt.Errorf("failed to parse legacy config: %w", err)
		}
		defaults := DefaultFileConfig()
		fc = &defaults
		legacy := convertLegacyConfig(&lc)
		if legacy.Server.Port != "" {
			fc.Server.Port = legacy.Server.Port
		}
		if legacy.Server.Token != "" {
			fc.Server.Token = legacy.Server.Token
		}
		if legacy.Server.StateDir != "" {
			fc.Server.StateDir = legacy.Server.StateDir
		}
		if legacy.InstanceDefaults.Mode != "" {
			fc.InstanceDefaults.Mode = legacy.InstanceDefaults.Mode
		}
		if legacy.InstanceDefaults.NoRestore != nil {
			fc.InstanceDefaults.NoRestore = legacy.InstanceDefaults.NoRestore
		}
		if legacy.InstanceDefaults.MaxTabs != nil {
			fc.InstanceDefaults.MaxTabs = legacy.InstanceDefaults.MaxTabs
		}
		if legacy.Profiles.BaseDir != "" {
			fc.Profiles.BaseDir = legacy.Profiles.BaseDir
		}
		if legacy.Profiles.DefaultProfile != "" {
			fc.Profiles.DefaultProfile = legacy.Profiles.DefaultProfile
		}
		if legacy.Security.AllowEvaluate != nil {
			fc.Security.AllowEvaluate = legacy.Security.AllowEvaluate
		}
		if legacy.Security.AllowMacro != nil {
			fc.Security.AllowMacro = legacy.Security.AllowMacro
		}
		if legacy.Security.AllowScreencast != nil {
			fc.Security.AllowScreencast = legacy.Security.AllowScreencast
		}
		if legacy.Security.AllowDownload != nil {
			fc.Security.AllowDownload = legacy.Security.AllowDownload
		}
		if legacy.Security.AllowUpload != nil {
			fc.Security.AllowUpload = legacy.Security.AllowUpload
		}
		if legacy.Timeouts.ActionSec != 0 {
			fc.Timeouts.ActionSec = legacy.Timeouts.ActionSec
		}
		if legacy.Timeouts.NavigateSec != 0 {
			fc.Timeouts.NavigateSec = legacy.Timeouts.NavigateSec
		}
		if legacy.MultiInstance.InstancePortStart != nil {
			fc.MultiInstance.InstancePortStart = legacy.MultiInstance.InstancePortStart
		}
		if legacy.MultiInstance.InstancePortEnd != nil {
			fc.MultiInstance.InstancePortEnd = legacy.MultiInstance.InstancePortEnd
		}
	} else {
		defaults := DefaultFileConfig()
		defaults.ConfigVersion = "" // Don't assume version — let file content determine it
		fc = &defaults
		if err := json.Unmarshal(data, fc); err != nil {
			return nil, configPath, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	return fc, configPath, nil
}

// SaveFileConfig saves a FileConfig to the specified path.
func SaveFileConfig(fc *FileConfig, path string) error {
	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	// Add trailing newline
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
