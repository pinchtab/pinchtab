// Package geo maps a proxy IP to geographic alignment data (timezone, locale, WebRTC IP, country)
// used to align browser fingerprint surfaces with the proxy's apparent location.
package geo

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// Provider is best-effort: implementations MUST handle empty IP by returning zero Info, nil.
type Provider interface {
	Lookup(ctx context.Context, ip string) (Info, error)
}

// Info carries fingerprint-alignment fields; zero-value fields mean "no opinion".
type Info struct {
	Timezone   string
	Locale     string
	WebRTCIP   string
	CountryISO string
}

func (i Info) IsZero() bool {
	return i == Info{}
}

type Noop struct{}

func (Noop) Lookup(context.Context, string) (Info, error) { return Info{}, nil }

// Static returns the configured Info for every call; driven by browser.proxy.geo.*.
type Static struct {
	Info Info
}

func (s Static) Lookup(context.Context, string) (Info, error) { return s.Info, nil }

var localeRegex = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)
var countryRegex = regexp.MustCompile(`^[A-Z]{2}$`)

// Validate skips zero-value fields so partial geo blocks are accepted.
func Validate(info Info) error {
	if tz := strings.TrimSpace(info.Timezone); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return fmt.Errorf("timezone %q is not a valid IANA zone: %w", tz, err)
		}
	}
	if loc := strings.TrimSpace(info.Locale); loc != "" {
		if !localeRegex.MatchString(loc) {
			return fmt.Errorf("locale %q must match BCP-47 form like \"en\" or \"en-GB\"", loc)
		}
	}
	if ip := strings.TrimSpace(info.WebRTCIP); ip != "" {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("webrtcIP %q is not a valid IP address", ip)
		}
	}
	if cc := strings.TrimSpace(info.CountryISO); cc != "" {
		if !countryRegex.MatchString(cc) {
			return fmt.Errorf("countryISO %q must be a 2-letter ISO 3166-1 alpha-2 code", cc)
		}
	}
	return nil
}
