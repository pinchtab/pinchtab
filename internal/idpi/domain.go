package idpi

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/security"
)

// CheckDomain evaluates rawURL against the domain whitelist in cfg.
//
// It returns a non-zero CheckResult when the feature is enabled, the whitelist
// is non-empty, and the host extracted from rawURL does not match any allowed
// pattern. The host-allowlist primitive lives in internal/security; this
// function layers IDPI's enabled/strictMode policy on top.
//
// Supported pattern forms (delegated to security):
//   - "example.com"  – exact host match (case-insensitive, port stripped)
//   - "*.example.com" – any single subdomain of example.com but NOT example.com
//   - "*"            – matches any host (effectively disables the whitelist)
func CheckDomain(rawURL string, cfg config.IDPIConfig, allowedDomains []string) CheckResult {
	if !cfg.Enabled || len(allowedDomains) == 0 {
		return CheckResult{}
	}
	if security.IsAllowedSpecialURL(rawURL) {
		return CheckResult{}
	}

	host := security.ExtractHost(rawURL)
	if host == "" {
		return makeResult(cfg.StrictMode,
			"URL has no domain component and cannot be verified against allowedDomains")
	}

	if security.HostMatchesPatterns(host, allowedDomains) {
		return CheckResult{}
	}

	return makeResult(cfg.StrictMode,
		fmt.Sprintf("domain %q is not in the allowed list (security.allowedDomains)", host))
}

// DomainAllowed reports whether rawURL's host matches an explicit allowedDomains
// entry under an active IDPI domain allowlist. Consumers feed it into navguard's
// allowExplicitInternal, so it answers "did the operator name THIS host", never
// "is this host permitted" — for the latter use security.HostAllowed.
//
// A bare "*" is dropped before matching: it denotes "anything", which is the same
// intent as an empty list, and an empty list grants nothing. It stays a full match
// for the domain RESTRICTION in CheckDomain — only the private-IP override is
// withheld from it. "*.corp.example.com" still denotes a set of hosts and is kept.
func DomainAllowed(rawURL string, cfg config.IDPIConfig, allowedDomains []string) bool {
	if !cfg.Enabled || security.IsAllowedSpecialURL(rawURL) {
		return false
	}
	named := hostDenotingPatterns(allowedDomains)
	if len(named) == 0 {
		return false
	}
	host := security.ExtractHost(rawURL)
	if host == "" {
		return false
	}
	return security.HostMatchesPatterns(host, named)
}

// hostDenotingPatterns drops the entries that name no host, so ["*", "example.com"]
// still grants for example.com and grants nothing for anything else.
func hostDenotingPatterns(allowedDomains []string) []string {
	named := make([]string, 0, len(allowedDomains))
	for _, pattern := range allowedDomains {
		if strings.TrimSpace(pattern) == "*" {
			continue
		}
		named = append(named, pattern)
	}
	return named
}

func makeResult(strictMode bool, reason string) CheckResult {
	return CheckResult{Threat: true, Blocked: strictMode, Reason: reason}
}
