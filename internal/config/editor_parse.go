package config

import (
	"fmt"
	"strings"
)

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q (use true/false)", s)
	}
}

func parseCSVList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// validateAllowlistEntries rejects obviously-invalid domain entries. It exists to
// catch a user pasting a remediation hint that contains the "…" placeholder
// literally (the allowlist would otherwise silently accept "…" as a host) and
// other malformed entries, rather than failing later at navigation time.
func validateAllowlistEntries(domains []string) error {
	for _, d := range domains {
		if strings.ContainsRune(d, '…') || strings.Contains(d, "...") {
			return fmt.Errorf("invalid domain %q: \"…\" is a placeholder, not a real host — replace it with an actual domain (e.g. example.com)", d)
		}
		if strings.ContainsAny(d, " \t/") {
			return fmt.Errorf("invalid domain %q: a domain must not contain spaces or '/' (use a bare host like example.com or *.example.com)", d)
		}
	}
	return nil
}
