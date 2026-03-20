package urls

import (
	"net"
	"net/url"
	"strings"
)

const (
	maxLogURLBytes    = 512
	logTruncateSuffix = "..."
)

// RedactForLog normalizes a URL for logs and strips sensitive components.
// It removes userinfo, query, and fragment and caps the final string length.
// Invalid inputs return an empty string rather than echoing raw potentially
// sensitive data back into logs.
func RedactForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	normalized, err := Sanitize(raw)
	if err != nil {
		normalized = raw
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" && parsed.Host == "" && parsed.Opaque == "" {
		return ""
	}

	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.ForceQuery = false

	if parsed.Host != "" {
		host := strings.ToLower(parsed.Hostname())
		if port := parsed.Port(); port != "" {
			parsed.Host = net.JoinHostPort(host, port)
		} else {
			parsed.Host = host
		}
	}

	return truncateForLog(parsed.String(), maxLogURLBytes)
}

func truncateForLog(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= len(logTruncateSuffix) {
		return logTruncateSuffix[:maxBytes]
	}

	limit := maxBytes - len(logTruncateSuffix)
	cut := 0
	for i := range s {
		if i > limit {
			break
		}
		cut = i
	}
	if cut == 0 && limit > 0 {
		return logTruncateSuffix
	}
	return s[:cut] + logTruncateSuffix
}
