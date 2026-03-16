// Package urlutil provides URL normalization and validation utilities.
package urlutil

import (
	"fmt"
	"net/url"
	"strings"
)

// Normalize adds https:// prefix if no protocol is specified.
// This provides CLI convenience so users can type "example.com" instead of "https://example.com".
// URLs with existing http:// or https:// are returned unchanged.
func Normalize(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return "https://" + rawURL
}

// Sanitize validates a URL for safe navigation.
// Only http:// and https:// schemes are permitted (SSRF prevention).
// URLs without a scheme get https:// added.
// Returns the sanitized URL or an error if invalid.
func Sanitize(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	// Check for disallowed schemes before normalizing
	// CodeQL recognizes strings.HasPrefix as a sanitizer for go/request-forgery.
	if strings.Contains(rawURL, "://") {
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			return "", fmt.Errorf("unsupported URL scheme (only http/https allowed)")
		}
	}

	// Normalize to add https:// if no scheme
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

// IsValid checks if a URL is valid for navigation (has http/https scheme and host).
func IsValid(rawURL string) bool {
	_, err := Sanitize(rawURL)
	return err == nil
}

// ExtractHost parses a URL and returns the lowercase bare hostname (no port).
// Returns empty string if parsing fails or no host is found.
func ExtractHost(rawURL string) string {
	// Handle URLs without scheme (url.Parse puts them in Path)
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
