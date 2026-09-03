package navguard

import (
	"fmt"
	"net/url"
	"strings"
)

// MaxURLLen is the maximum allowed navigation URL length (8 KiB).
const MaxURLLen = 8 << 10

// ValidateURL checks that raw is a well-formed, scheme-safe navigation URL.
// It allows http, https, about:blank, and bare hostnames (no scheme).
//
// Not a Validator method: the answer depends on nothing a Validator carries, and
// hanging it off one implied that TrustedResolveCIDRs applied to it.
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
