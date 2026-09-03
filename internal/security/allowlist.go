// Package security owns the cross-cutting host allowlist primitive sourced
// from security.allowedDomains in the file config. Feature-specific consumers
// (IDPI, route interception, download policy, ...) import this package; the
// reverse must not happen.
package security

import (
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/urls"
)

// HostAllowed reports whether rawURL's host matches an entry in allowedDomains.
//
// An empty allowedDomains list means "no allowlist active" → returns true.
// A non-empty list with no matching host → returns false. Special non-routable
// URLs ("about:blank") are always allowed because they have no host to verify.
//
// Supported pattern forms:
//   - "example.com"   – exact host match (case-insensitive, port stripped)
//   - "*.example.com" – any single subdomain of example.com but NOT example.com
//   - "*"             – matches any host (effectively disables the allowlist)
func HostAllowed(rawURL string, allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return true
	}
	if isAllowedSpecialURL(rawURL) {
		return true
	}
	host := ExtractHost(rawURL)
	if host == "" {
		return false
	}
	return HostMatchesPatterns(host, allowedDomains)
}

// HostMatchesPatterns reports whether host (already lowercased) matches any of
// the patterns. Exposed so callers that already have a host don't have to
// re-parse a URL.
func HostMatchesPatterns(host string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if matchDomain(host, pattern) {
			return true
		}
	}
	return false
}

// urls.EnsureScheme owns the normalisation that decides which forms carry a host.
func ExtractHost(rawURL string) string {
	parsed, err := url.Parse(urls.EnsureScheme(rawURL))
	if err != nil {
		return ""
	}
	// The DNS root label is silent: "example.com." and "example.com" name the
	// same host and the browser fetches both. Leaving it on made two answers
	// differ, and this primitive is read in both directions — a listed host that
	// stops matching is a refusal on a target the operator allowed, and the
	// response-forgery rule reads the same answer inverted, where a listed host
	// that stops matching PERMITS forgery on exactly the sensitive origin the
	// list marks.
	return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
}

// IsAllowedSpecialURL reports whether rawURL is a non-routable URL that bypasses
// host-based allowlist checks (e.g. "about:blank").
func IsAllowedSpecialURL(rawURL string) bool {
	return isAllowedSpecialURL(rawURL)
}

func isAllowedSpecialURL(rawURL string) bool {
	return strings.EqualFold(strings.TrimSpace(rawURL), "about:blank")
}

func matchDomain(host, pattern string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*."):
		suffix := pattern[1:]
		return strings.HasSuffix(host, suffix)
	default:
		return host == pattern
	}
}
