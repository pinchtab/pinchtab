package idpi

import "github.com/pinchtab/pinchtab/internal/config"

// BuiltinGuard is the original hand-rolled implementation.
// It uses substring matching with basic normalization.
type BuiltinGuard struct {
	cfg config.IDPIConfig
}

// NewBuiltinGuard creates a guard backed by the built-in pattern scanner.
func NewBuiltinGuard(cfg config.IDPIConfig) *BuiltinGuard {
	return &BuiltinGuard{cfg: cfg}
}

func (g *BuiltinGuard) Enabled() bool { return g.cfg.Enabled }

func (g *BuiltinGuard) ScanContent(text string) CheckResult {
	return ScanContent(text, g.cfg)
}

func (g *BuiltinGuard) CheckDomain(rawURL string) CheckResult {
	return CheckDomain(rawURL, g.cfg)
}

func (g *BuiltinGuard) DomainAllowed(rawURL string) bool {
	return DomainAllowed(rawURL, g.cfg)
}

func (g *BuiltinGuard) WrapContent(text, pageURL string) string {
	return WrapContent(text, pageURL)
}
