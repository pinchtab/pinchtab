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
	if cfg := o.cfg(); cfg != nil {
		host = configuredChildInstanceHost(cfg.Bind)
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

func (o *Orchestrator) baseChildFileConfig(effectiveCfg *config.RuntimeConfig, port, stateDir string) config.FileConfig {
	if effectiveCfg == nil {
		effectiveCfg = o.cfg()
	}
	fc := config.FileConfigFromRuntime(effectiveCfg)
	fc.Server.Port = port
	fc.Server.StateDir = stateDir
	activityEnabled := false
	fc.Observability.Activity.Enabled = &activityEnabled
	return fc
}

func (o *Orchestrator) buildChildFileConfig(effectiveCfg *config.RuntimeConfig, port string, cdpPort int, profilePath, instanceStateDir string, headless bool, extensionPaths []string, securityPolicy *bridge.SecurityPolicy) config.FileConfig {
	fc := o.baseChildFileConfig(effectiveCfg, port, instanceStateDir)
	fc.SetBrowserDebugPort(cdpPort)
	fc.Profiles.BaseDir = filepath.Dir(profilePath)
	fc.Profiles.DefaultProfile = filepath.Base(profilePath)
	fc.InstanceDefaults.Mode = "headed"
	if headless {
		fc.InstanceDefaults.Mode = "headless"
	}
	if securityPolicy != nil {
		fc.Security.AllowedDomains = append([]string(nil), securityPolicy.AllowedDomains...)
	}
	if len(extensionPaths) > 0 {
		fc.Browser.ExtensionPaths = uniqueStrings(fc.Browser.ExtensionPaths, extensionPaths)
	}
	return fc
}

func (o *Orchestrator) writeChildConfig(effectiveCfg *config.RuntimeConfig, port string, cdpPort int, profilePath, instanceStateDir string, headless bool, extensionPaths []string, securityPolicy *bridge.SecurityPolicy) (string, error) {
	return writeChildConfigFile(instanceStateDir, o.buildChildFileConfig(effectiveCfg, port, cdpPort, profilePath, instanceStateDir, headless, extensionPaths, securityPolicy))
}

func (o *Orchestrator) writeAttachChildConfig(port, provider, stateDir string) (string, error) {
	fc := o.baseChildFileConfig(nil, port, stateDir)
	fc.Browsers.Default = provider
	attachDisabled := false
	fc.Security.Attach = config.AttachConfig{
		Enabled:          &attachDisabled,
		AllowHosts:       append([]string(nil), fc.Security.Attach.AllowHosts...),
		AllowSchemes:     append([]string(nil), fc.Security.Attach.AllowSchemes...),
		ForwardProxyAuth: &attachDisabled,
	}
	return writeChildConfigFile(stateDir, fc)
}

func writeChildConfigFile(stateDir string, fc config.FileConfig) (string, error) {
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
	return uniqueStrings(trimAll(base), trimAll(extras))
}

func trimAll(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func uniqueStrings(lists ...[]string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, list := range lists {
		for _, value := range list {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func effectiveBinaryFromCfg(cfg *config.RuntimeConfig) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.BrowserBinary)
}
