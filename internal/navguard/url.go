package navguard

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// MaxURLLen is the maximum allowed navigation URL length (8 KiB).
const MaxURLLen = 8 << 10

// ValidateURL checks that raw is a well-formed, scheme-safe navigation URL.
// It allows http, https, about:blank, and bare hostnames (no scheme).
func (v *Validator) ValidateURL(raw string) error {
	return ValidateURL(raw)
}

// ValidateURL is the standalone (non-method) URL validator, usable without a
// Validator instance.
func ValidateURL(raw string) error {
	return ValidateURLAllowingFile(raw, false)
}

func ValidateURLAllowingFile(raw string, allowFile bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("url required")
	}
	if len(raw) > MaxURLLen {
		return fmt.Errorf("url too long")
	}
	if strings.EqualFold(raw, "about:blank") {
		return nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if parsed.Scheme == "" {
		return nil
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	case "file":
		if allowFile {
			return nil
		}
		return fmt.Errorf("invalid URL scheme: %s", parsed.Scheme)
	default:
		return fmt.Errorf("invalid URL scheme: %s", parsed.Scheme)
	}
}

func IsFileURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "file")
}

// ExtractHost extracts the hostname from a raw URL string.
// Returns the lowercase, dot-trimmed host and true if found.
func ExtractHost(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), "."); host != "" {
		return host, true
	}
	if parsed.Scheme != "" {
		return "", false
	}

	bare := parsed.Path
	bare = strings.SplitN(bare, "/", 2)[0]
	bare = strings.SplitN(bare, "?", 2)[0]
	bare = strings.SplitN(bare, "#", 2)[0]
	bare = strings.TrimSpace(bare)
	if bare == "" || strings.HasPrefix(bare, "/") || strings.HasPrefix(bare, ".") {
		return "", false
	}
	if h, _, err := net.SplitHostPort(bare); err == nil {
		bare = h
	}
	host := strings.TrimSuffix(strings.ToLower(bare), ".")
	if host == "" {
		return "", false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil || strings.Contains(host, ".") || strings.Contains(host, ":") {
		return host, true
	}
	return "", false
}
