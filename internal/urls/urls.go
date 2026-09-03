// Package urls provides URL normalization and validation utilities.
package urls

import (
	"fmt"
	"net/url"
	"strings"
)

// Normalize adds https:// if no protocol specified. Existing http/https preserved.
func Normalize(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return "https://" + rawURL
}

// Sanitize normalizes a URL. Bare hostnames get https:// added.
// All explicit schemes are passed through (user knows what they're doing).
func Sanitize(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	normalized := EnsureScheme(rawURL)
	if normalized == rawURL {
		return rawURL, nil
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("missing host in URL")
	}

	return parsed.String(), nil
}

// IsValid returns true if URL is safe for navigation.
func IsValid(rawURL string) bool {
	_, err := Sanitize(rawURL)
	return err == nil
}

// EnsureScheme is the ONE place a scheme-less target acquires one, so no consumer
// has to hand-roll a bare-URL branch to find the host inside it. url.Parse reads a
// scheme-less string's leading label as a scheme, which is right for the opaque
// forms ("about:blank", "data:...", "javascript:...") and wrong for "intranet:8080",
// where the label is a host and the opaque part is a port — that is the one case
// this has to tell apart, and a port is what tells it.
//
// https:// is the default a bare host gets. It is the assumption Normalize already
// makes for the CLI and Sanitize for MCP, and it is the safer of the two: a target
// written without a scheme is upgraded rather than downgraded.
func EnsureScheme(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return trimmed
	}
	if strings.Contains(trimmed, "://") {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && !opaqueStartsWithPort(parsed.Opaque) {
		return trimmed
	}
	return "https://" + trimmed
}

// opaqueStartsWithPort reports whether an opaque remainder begins with a port —
// "8080" or "8080/path" — which is what makes "intranet:8080" a host and a port
// rather than a scheme and a body.
func opaqueStartsWithPort(opaque string) bool {
	digits := strings.SplitN(opaque, "/", 2)[0]
	if digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
