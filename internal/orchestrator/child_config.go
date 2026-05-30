package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func (o *Orchestrator) childInstanceBaseURL(port string) string {
	host := configuredChildInstanceHost("")
	if o != nil && o.runtimeCfg != nil {
		host = configuredChildInstanceHost(o.runtimeCfg.Bind)
	}
	return httpBaseURL(host, port)
}

func portConflictError(port string, inspection PortInspection) error {
	if inspection.PID > 0 {
		process := fmt.Sprintf("pid %d", inspection.PID)
		if command := strings.TrimSpace(inspection.Command); command != "" {
			process = fmt.Sprintf("%s (%s)", process, command)
		}
		if strings.Contains(strings.ToLower(inspection.Command), "pinchtab") {
			return fmt.Errorf("instance port %s is already in use by %s; stop the stale process and restart PinchTab, for example: kill %d", port, process, inspection.PID)
		}
		return fmt.Errorf("instance port %s is already in use by %s; stop the process and restart PinchTab, for example: kill %d", port, process, inspection.PID)
	}
	return fmt.Errorf("instance port %s is already in use on this machine", port)
}

// buildChildFileConfig builds the per-child FileConfig; nil effectiveCfg falls back to o.runtimeCfg.
func (o *Orchestrator) buildChildFileConfig(effectiveCfg *config.RuntimeConfig, port string, cdpPort int, profilePath, instanceStateDir string, headless bool, extensionPaths []string, securityPolicy *bridge.SecurityPolicy) config.FileConfig {
	if effectiveCfg == nil {
		effectiveCfg = o.runtimeCfg
	}
	fc := config.FileConfigFromRuntime(effectiveCfg)
	fc.Server.Port = port
	fc.Server.StateDir = instanceStateDir
	activityEnabled := false
	fc.Observability.Activity.Enabled = &activityEnabled
	fc.Browser.ChromeDebugPort = intPtr(cdpPort)
	fc.Profiles.BaseDir = filepath.Dir(profilePath)
	fc.Profiles.DefaultProfile = filepath.Base(profilePath)
	if headless {
		fc.InstanceDefaults.Mode = "headless"
	} else {
		fc.InstanceDefaults.Mode = "headed"
	}
	if securityPolicy != nil {
		fc.Security.AllowedDomains = append([]string(nil), securityPolicy.AllowedDomains...)
	}

	if len(extensionPaths) > 0 {
		seen := make(map[string]bool)
		unique := make([]string, 0, len(fc.Browser.ExtensionPaths)+len(extensionPaths))
		for _, p := range fc.Browser.ExtensionPaths {
			if !seen[p] {
				seen[p] = true
				unique = append(unique, p)
			}
		}
		for _, p := range extensionPaths {
			if !seen[p] {
				seen[p] = true
				unique = append(unique, p)
			}
		}
		fc.Browser.ExtensionPaths = unique
	}
	return fc
}

func (o *Orchestrator) writeChildConfig(effectiveCfg *config.RuntimeConfig, port string, cdpPort int, profilePath, instanceStateDir string, headless bool, extensionPaths []string, securityPolicy *bridge.SecurityPolicy) (string, error) {
	fc := o.buildChildFileConfig(effectiveCfg, port, cdpPort, profilePath, instanceStateDir, headless, extensionPaths, securityPolicy)

	configPath := filepath.Join(instanceStateDir, "config.json")
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		return "", err
	}
	return configPath, nil
}

// writeAttachChildConfig writes a minimal child config for a CDP-attach bridge; RemoteCDPURL is passed via CLI flags.
func (o *Orchestrator) writeAttachChildConfig(port, provider, stateDir string) (string, error) {
	fc := config.FileConfigFromRuntime(o.runtimeCfg)
	fc.Server.Port = port
	fc.Server.StateDir = stateDir
	activityEnabled := false
	fc.Observability.Activity.Enabled = &activityEnabled
	fc.Browsers.Default = provider
	attachDisabled := false
	allowHosts := append([]string(nil), fc.Security.Attach.AllowHosts...)
	allowSchemes := append([]string(nil), fc.Security.Attach.AllowSchemes...)
	fc.Security.Attach = config.AttachConfig{
		Enabled:          &attachDisabled,
		AllowHosts:       allowHosts,
		AllowSchemes:     allowSchemes,
		ForwardProxyAuth: &attachDisabled,
	}

	configPath := filepath.Join(stateDir, "config.json")
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		return "", err
	}
	return configPath, nil
}

func effectiveSecurityPolicy(cfg *config.RuntimeConfig, requested *bridge.SecurityPolicy) *bridge.SecurityPolicy {
	var merged []string
	if cfg != nil {
		merged = mergeAllowedDomains(merged, cfg.AllowedDomains)
	}
	if requested != nil {
		merged = mergeAllowedDomains(merged, requested.AllowedDomains)
	}
	if len(merged) == 0 {
		return nil
	}
	return &bridge.SecurityPolicy{AllowedDomains: merged}
}

func cloneSecurityPolicy(policy *bridge.SecurityPolicy) *bridge.SecurityPolicy {
	if policy == nil {
		return nil
	}
	return &bridge.SecurityPolicy{
		AllowedDomains: append([]string(nil), policy.AllowedDomains...),
	}
}

func mergeAllowedDomains(base []string, extras []string) []string {
	seen := make(map[string]bool, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, domain := range base {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	for _, domain := range extras {
		trimmed := strings.TrimSpace(domain)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func effectiveBinaryFromCfg(cfg *config.RuntimeConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.ChromeBinary
}

func intPtr(v int) *int {
	if v <= 0 {
		return nil
	}
	n := v
	return &n
}
