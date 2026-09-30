package config

import "strings"

func ApplyTargetOverride(cfg *RuntimeConfig, targetName string) *RuntimeConfig {
	if cfg == nil {
		return cfg
	}
	targetName = strings.TrimSpace(targetName)
	if targetName == "" {
		return cfg
	}
	resolved, err := ResolveExplicitBrowserTarget(cfg, targetName)
	if err != nil {
		return cfg
	}
	return resolved.Config
}

func ResolveRequestedTarget(cfg *RuntimeConfig, requested string) (targetName, provider string, err error) {
	var resolved *ResolvedBrowserTarget
	if strings.TrimSpace(requested) == "" {
		resolved, err = ResolveDefaultBrowserTarget(cfg)
	} else {
		resolved, err = ResolveExplicitBrowserTarget(cfg, requested)
	}
	if err != nil || resolved == nil {
		return "", "", err
	}
	if resolved.Legacy {
		return "", "", nil
	}
	return resolved.Name, resolved.Provider, nil
}
