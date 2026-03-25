package idpi

import (
	"github.com/pinchtab/idpishield"
	"github.com/pinchtab/pinchtab/internal/config"
)

// ShieldGuard uses the idpishield library for content scanning
// while delegating domain checks and wrapping to the built-in logic.
type ShieldGuard struct {
	shield  *idpishield.Shield
	builtin *BuiltinGuard
	cfg     config.IDPIConfig
}

// NewShieldGuard creates a guard backed by idpishield for content analysis.
func NewShieldGuard(cfg config.IDPIConfig) *ShieldGuard {
	mode := idpishield.ModeBalanced
	if cfg.StrictMode {
		mode = idpishield.ModeDeep
	}

	shield := idpishield.New(idpishield.Config{
		Mode:           mode,
		AllowedDomains: cfg.AllowedDomains,
		StrictMode:     cfg.StrictMode,
	})

	return &ShieldGuard{
		shield:  shield,
		builtin: NewBuiltinGuard(cfg),
		cfg:     cfg,
	}
}

func (g *ShieldGuard) Enabled() bool { return g.cfg.Enabled }

func (g *ShieldGuard) ScanContent(text string) CheckResult {
	if !g.cfg.Enabled || !g.cfg.ScanContent || text == "" {
		return CheckResult{}
	}

	result := g.shield.Assess(text, "")

	cr := CheckResult{
		Threat:  result.Score >= 40,
		Blocked: result.Blocked,
		Reason:  result.Reason,
	}

	if len(result.Patterns) > 0 {
		cr.Pattern = result.Patterns[0]
	}

	// Also check custom patterns (idpishield doesn't know about them).
	if !cr.Threat && len(g.cfg.CustomPatterns) > 0 {
		if br := ScanContent(text, g.cfg); br.Threat {
			return br
		}
	}

	return cr
}

func (g *ShieldGuard) CheckDomain(rawURL string) CheckResult {
	return g.builtin.CheckDomain(rawURL)
}

func (g *ShieldGuard) DomainAllowed(rawURL string) bool {
	return g.builtin.DomainAllowed(rawURL)
}

func (g *ShieldGuard) WrapContent(text, pageURL string) string {
	return g.builtin.WrapContent(text, pageURL)
}
