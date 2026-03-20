package activity

import (
	"net"
	"net/url"
	"strings"

	internalurls "github.com/pinchtab/pinchtab/internal/urls"
)

const (
	maxActivityURLBytes    = 2048
	activityTruncateSuffix = "..."
)

func sanitizeActivityURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	normalized, err := internalurls.Sanitize(raw)
	if err != nil {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || (parsed.Scheme == "" && parsed.Host == "" && parsed.Opaque == "") {
			return ""
		}
		normalized = raw
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
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

	return truncateActivityValue(parsed.String(), maxActivityURLBytes)
}

func truncateActivityValue(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= len(activityTruncateSuffix) {
		return activityTruncateSuffix[:maxBytes]
	}

	limit := maxBytes - len(activityTruncateSuffix)
	cut := 0
	for i := range s {
		if i > limit {
			break
		}
		cut = i
	}
	if cut == 0 && limit > 0 {
		return activityTruncateSuffix
	}
	return s[:cut] + activityTruncateSuffix
}
