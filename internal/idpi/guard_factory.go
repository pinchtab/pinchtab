package idpi

import "github.com/pinchtab/pinchtab/internal/config"

// NewGuard creates the appropriate Guard implementation based on config.
// Returns a ShieldGuard when IDPI is enabled, noopGuard otherwise.
func NewGuard(cfg config.IDPIConfig) Guard {
	if !cfg.Enabled {
		return noopGuard{}
	}
	return NewShieldGuard(cfg)
}

// noopGuard is a Guard that does nothing (IDPI disabled).
type noopGuard struct{}

func (noopGuard) Enabled() bool                     { return false }
func (noopGuard) ScanContent(string) CheckResult    { return CheckResult{} }
func (noopGuard) CheckDomain(string) CheckResult    { return CheckResult{} }
func (noopGuard) DomainAllowed(string) bool         { return false }
func (noopGuard) WrapContent(text, _ string) string { return text }
