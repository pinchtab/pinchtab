// Package urlutil provides URL normalization and validation utilities.
package urlutil

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

	// If URL has an explicit scheme, pass it through unchanged
	if strings.Contains(rawURL, "://") ||
		strings.HasPrefix(rawURL, "javascript:") ||
		strings.HasPrefix(rawURL, "vbscript:") ||
		strings.HasPrefix(rawURL, "about:") ||
		strings.HasPrefix(rawURL, "data:") {
		return rawURL, nil
	}

	// Bare hostname — normalize to https://
	normalized := Normalize(rawURL)

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

// ExtractHost returns the lowercase hostname without port. Empty string on failure.
func ExtractHost(rawURL string) string {
	// url.Parse puts bare hostnames into Path when no scheme is present
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	host := parsed.Hostname() // strips port
	return strings.ToLower(host)
}
