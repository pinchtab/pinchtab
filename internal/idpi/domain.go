package idpi

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

// CheckDomain evaluates rawURL against the domain whitelist in cfg.
//
// It returns a non-zero CheckResult when the feature is enabled, the whitelist
// is non-empty, and the host extracted from rawURL does not match any allowed
// pattern.
//
// Supported pattern forms:
//   - "example.com"  – exact host match (case-insensitive, port stripped)
//   - "*.example.com" – matches any single subdomain of example.com but NOT
//     example.com itself
//   - "*"            – matches any host (effectively disables the whitelist)
func CheckDomain(rawURL string, cfg config.IDPIConfig) CheckResult {
	if !cfg.Enabled || len(cfg.AllowedDomains) == 0 {
		return CheckResult{}
	}
	if isAllowedSpecialURL(rawURL) {
		return CheckResult{}
	}

	host := extractHost(rawURL)
	if host == "" {
		// Cannot extract a domain component (e.g. file://, about:blank, chrome://).
		// When a whitelist is active we must not silently allow URLs we cannot
		// verify — deny them so callers can decide how to proceed.
		return makeResult(cfg.StrictMode,
			"URL has no domain component and cannot be verified against allowedDomains")
	}

	for _, pattern := range cfg.AllowedDomains {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if matchDomain(host, pattern) {
			return CheckResult{}
		}
	}

	return makeResult(cfg.StrictMode,
		fmt.Sprintf("domain %q is not in the allowed list (security.idpi.allowedDomains)", host))
}

func isAllowedSpecialURL(rawURL string) bool {
	return strings.EqualFold(strings.TrimSpace(rawURL), "about:blank")
}

// extractHost parses rawURL and returns the lowercase bare hostname (no port).
// It handles both fully-qualified URLs ("https://example.com:8080/path") and
// bare hostnames ("example.com" or "example.com/path").
func extractHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	host := parsed.Hostname() // strips port; returns "" for bare hostnames

	if host == "" {
		// url.Parse puts bare hostnames into Path when no scheme is present.
		// Recover the host by treating the first path segment as the authority.
		bare := parsed.Path
		bare = strings.SplitN(bare, "/", 2)[0]
		bare = strings.SplitN(bare, "?", 2)[0]
		bare = strings.SplitN(bare, "#", 2)[0]
		if h, _, err := net.SplitHostPort(bare); err == nil {
			host = h
		} else {
			host = bare
		}
	}

	return strings.ToLower(strings.TrimSpace(host))
}

// matchDomain reports whether host matches pattern (both already lowercased).
func matchDomain(host, pattern string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*."):
		// "*.example.com" matches "foo.example.com" but NOT "example.com"
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix)
	default:
		return host == pattern
	}
}

func makeResult(strictMode bool, reason string) CheckResult {
	return CheckResult{Threat: true, Blocked: strictMode, Reason: reason}
}
