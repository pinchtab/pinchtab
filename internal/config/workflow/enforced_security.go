package workflow

import "github.com/pinchtab/pinchtab/internal/config"

// EnforcedSecurity is the security posture one process enforces: the policy it
// snapshotted from the config it booted with, which a front door's current
// configuration does not speak for.
type EnforcedSecurity struct {
	IDPIEnabled               bool     `json:"idpiEnabled"`
	AllowedDomains            []string `json:"allowedDomains"`
	EnabledSensitiveEndpoints []string `json:"enabledSensitiveEndpoints"`
	GuardsDown                bool     `json:"guardsDown"`
}

func EnforcedSecurityFor(cfg *config.RuntimeConfig) EnforcedSecurity {
	if cfg == nil {
		return EnforcedSecurity{}
	}
	return EnforcedSecurity{
		IDPIEnabled:               cfg.IDPI.Enabled,
		AllowedDomains:            append([]string{}, cfg.AllowedDomains...),
		EnabledSensitiveEndpoints: append([]string{}, cfg.EnabledSensitiveEndpoints()...),
		GuardsDown:                GuardsDownPostureActive(cfg),
	}
}
