package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config/geo"
)

// BrowserProxyConfig describes a single browser-level proxy.
//
// Server is "<scheme>://<host>:<port>" (http, https, socks4, socks5); embedded
// credentials are rejected — use Username/Password. Empty Server disables the
// proxy and ignores other fields. Proxy auth is delivered via CDP at the
// bridge layer (see internal/bridge/runtime/proxy_auth.go) because Chrome
// rejects credentials in --proxy-server.
type BrowserProxyConfig struct {
	Server     string                 `json:"server,omitempty"`
	BypassList []string               `json:"bypassList,omitempty"`
	Username   string                 `json:"username,omitempty"`
	Password   string                 `json:"password,omitempty"`
	Geo        *BrowserProxyGeoConfig `json:"geo,omitempty"`
}

// Zero-value fields mean "no opinion".
type BrowserProxyGeoConfig struct {
	Timezone   string `json:"timezone,omitempty"`
	Locale     string `json:"locale,omitempty"`
	WebRTCIP   string `json:"webrtcIP,omitempty"`
	CountryISO string `json:"countryISO,omitempty"`
}

func (g BrowserProxyGeoConfig) IsZero() bool {
	return g == BrowserProxyGeoConfig{}
}

var validProxySchemes = map[string]struct{}{
	"http":   {},
	"https":  {},
	"socks4": {},
	"socks5": {},
}

func (p BrowserProxyConfig) IsZero() bool {
	return strings.TrimSpace(p.Server) == ""
}

// Redacted keeps empty Password empty so callers can distinguish absent vs hidden.
func (p BrowserProxyConfig) Redacted() BrowserProxyConfig {
	out := BrowserProxyConfig{
		Server:   p.Server,
		Username: p.Username,
		Password: p.Password,
	}
	if len(p.BypassList) > 0 {
		out.BypassList = append([]string(nil), p.BypassList...)
	}
	if p.Geo != nil {
		geoCopy := *p.Geo
		out.Geo = &geoCopy
	}
	if p.Password != "" {
		out.Password = "***"
	}
	return out
}

func (p BrowserProxyConfig) GeoInfo() geo.Info {
	if p.Geo == nil {
		return geo.Info{}
	}
	return geo.Info{
		Timezone:   p.Geo.Timezone,
		Locale:     p.Geo.Locale,
		WebRTCIP:   p.Geo.WebRTCIP,
		CountryISO: p.Geo.CountryISO,
	}
}

// ParseProxyServer splits a proxy server URL strictly; rejects embedded userinfo to prevent credential leak.
func ParseProxyServer(server string) (scheme, host string, port int, err error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", "", 0, fmt.Errorf("proxy server is empty")
	}
	idx := strings.Index(server, "://")
	if idx <= 0 {
		return "", "", 0, fmt.Errorf("proxy server %q must include a scheme (http, https, socks4, socks5)", server)
	}
	scheme = strings.ToLower(server[:idx])
	if _, ok := validProxySchemes[scheme]; !ok {
		return "", "", 0, fmt.Errorf("proxy server scheme %q is not supported (use http, https, socks4, or socks5)", scheme)
	}
	rest := server[idx+3:]
	if rest == "" {
		return "", "", 0, fmt.Errorf("proxy server %q is missing host:port", server)
	}
	// Strip any path before the credential check: an '@' in the path
	// (http://host:8080/p@th) is not embedded userinfo.
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	if strings.ContainsRune(rest, '@') {
		return "", "", 0, fmt.Errorf("proxy server %q must not include embedded credentials; use the username/password fields", server)
	}
	h, p, splitErr := net.SplitHostPort(rest)
	if splitErr != nil {
		return "", "", 0, fmt.Errorf("proxy server %q must be of the form scheme://host:port: %w", server, splitErr)
	}
	if strings.TrimSpace(h) == "" {
		return "", "", 0, fmt.Errorf("proxy server %q has empty host", server)
	}
	portNum, atoiErr := strconv.Atoi(p)
	if atoiErr != nil {
		return "", "", 0, fmt.Errorf("proxy server %q port %q is not numeric: %w", server, p, atoiErr)
	}
	if portNum < 1 || portNum > 65535 {
		return "", "", 0, fmt.Errorf("proxy server %q port %d out of range (1-65535)", server, portNum)
	}
	return scheme, h, portNum, nil
}

// ValidateBrowserProxy returns nil when disabled (empty Server).
func ValidateBrowserProxy(field string, p BrowserProxyConfig) []error {
	if p.IsZero() {
		return nil
	}
	var errs []error

	if _, _, _, err := ParseProxyServer(p.Server); err != nil {
		errs = append(errs, ValidationError{
			Field:   field + ".server",
			Message: err.Error(),
		})
	}

	for i, pat := range p.BypassList {
		trimmed := strings.TrimSpace(pat)
		if trimmed == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.bypassList[%d]", field, i),
				Message: "bypass pattern must not be empty",
			})
			continue
		}
		if strings.ContainsAny(trimmed, " \t\n\r;") {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("%s.bypassList[%d]", field, i),
				Message: fmt.Sprintf("bypass pattern %q must not contain whitespace or ';' (use multiple entries)", trimmed),
			})
		}
	}

	if p.Geo != nil && !p.Geo.IsZero() {
		if err := geo.Validate(p.GeoInfo()); err != nil {
			errs = append(errs, ValidationError{
				Field:   field + ".geo",
				Message: err.Error(),
			})
		}
	}

	hasUser := strings.TrimSpace(p.Username) != ""
	hasPass := p.Password != ""
	if hasUser && !hasPass {
		errs = append(errs, ValidationError{
			Field:   field + ".password",
			Message: "password is required when username is set",
		})
	}
	if hasPass && !hasUser {
		errs = append(errs, ValidationError{
			Field:   field + ".username",
			Message: "username is required when password is set",
		})
	}

	return errs
}

// BrowserProxyFlags returns credential-free flags; auth is delivered via CDP.
// A malformed server is a hard error: launching without the configured proxy
// would silently egress traffic from the real IP — the worst failure mode for
// users who configured a proxy for anonymity.
func BrowserProxyFlags(p BrowserProxyConfig) ([]string, error) {
	if p.IsZero() {
		return nil, nil
	}
	scheme, host, port, err := ParseProxyServer(p.Server)
	if err != nil {
		return nil, fmt.Errorf("browser.proxy.server is invalid; refusing to launch without the configured proxy (traffic would egress directly): %w", err)
	}
	authority := net.JoinHostPort(host, strconv.Itoa(port))
	flags := []string{
		fmt.Sprintf("--proxy-server=%s://%s", scheme, authority),
	}
	if bypass := joinBypassList(p.BypassList); bypass != "" {
		flags = append(flags, "--proxy-bypass-list="+bypass)
	}
	return flags, nil
}

func joinBypassList(items []string) string {
	cleaned := make([]string, 0, len(items))
	for _, it := range items {
		trimmed := strings.TrimSpace(it)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	return strings.Join(cleaned, ";")
}
