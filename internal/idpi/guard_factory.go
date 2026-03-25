package idpi

import "github.com/pinchtab/pinchtab/internal/config"

// NewGuard creates the appropriate Guard implementation based on config.
// Returns a ShieldGuard (idpishield-backed) by default when IDPI is enabled,
// falling back to BuiltinGuard if the shield cannot be created.
func NewGuard(cfg config.IDPIConfig) Guard {
	if !cfg.Enabled {
		return NewBuiltinGuard(cfg)
	}
	return NewShieldGuard(cfg)
}
