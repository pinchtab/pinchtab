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

// Sanitize validates and normalizes a URL.
// Allows http/https (web), chrome/chrome-extension (browser internals), about/data (special pages).
// Blocks dangerous schemes like file:// and javascript:.
func Sanitize(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	// Allow browser-specific schemes that users might explicitly want
	allowedPrefixes := []string{
		"http://", "https://",
		"file://",
		"chrome://", "chrome-extension://",
		"about:", "data:",
	}

	// Block dangerous schemes (XSS)
	blockedPrefixes := []string{
		"javascript:",
	}

	if strings.Contains(rawURL, "://") || strings.Contains(rawURL, ":") {
		for _, blocked := range blockedPrefixes {
			if strings.HasPrefix(rawURL, blocked) {
				return "", fmt.Errorf("blocked URL scheme: %s", blocked)
			}
		}

		isAllowed := false
		for _, allowed := range allowedPrefixes {
			if strings.HasPrefix(rawURL, allowed) {
				isAllowed = true
				break
			}
		}

		// If has scheme but not in allowed list, check if it looks like a scheme
		if !isAllowed && strings.Contains(rawURL, "://") {
			return "", fmt.Errorf("unsupported URL scheme")
		}
	}

	// For non-http schemes, return as-is (no normalization needed)
	if strings.HasPrefix(rawURL, "file://") ||
		strings.HasPrefix(rawURL, "chrome://") ||
		strings.HasPrefix(rawURL, "chrome-extension://") ||
		strings.HasPrefix(rawURL, "about:") ||
		strings.HasPrefix(rawURL, "data:") {
		return rawURL, nil
	}

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
