package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const cdpVersionBodyLimit = 64 * 1024

var defaultCDPAllowHosts = []string{"127.0.0.1", "localhost", "::1"}
var defaultCDPAllowSchemes = []string{"ws", "wss", "http", "https"}

var cdpResolveHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, network, host)
}

type CDPVersionProbe struct {
	VersionURL           string
	WebSocketDebuggerURL string
}

// NormalizeCDPURL accepts any of:
//   - browser-level WebSocket URL: ws://host:port/devtools/browser/<id>
//   - HTTP DevTools origin:        http://host:port (or http://host:port/)
//   - HTTP /json/version URL:      http://host:port/json/version
//
// It returns the browser-level WebSocket URL suitable for chromedp.NewRemoteAllocator.
//
// Page-level URLs (containing /devtools/page/) are rejected in this slice.
func NormalizeCDPURL(raw string) (string, error) {
	return NormalizeCDPURLWithAllowedHosts(raw, defaultCDPAllowHosts)
}

// NormalizeCDPURLWithAllowedHosts resolves and pins the endpoint host. Loopback
// CDP endpoints are always accepted; non-loopback endpoints must match
// allowedHosts (or "*"). HTTP probes never follow redirects and response bodies
// are capped.
func NormalizeCDPURLWithAllowedHosts(raw string, allowedHosts []string) (string, error) {
	return NormalizeCDPURLWithAllowlist(raw, allowedHosts, defaultCDPAllowSchemes)
}

// NormalizeCDPURLWithAllowlist resolves and pins the endpoint host while
// enforcing the caller's attach host and input-scheme allowlists.
func NormalizeCDPURLWithAllowlist(raw string, allowedHosts, allowedSchemes []string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("cdpUrl is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid cdpUrl: %w", err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("cdpUrl must include a host")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("cdpUrl must not include credentials")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if err := validateCDPScheme(scheme, allowedSchemes); err != nil {
		return "", err
	}
	switch scheme {
	case "ws", "wss":
		if strings.Contains(parsed.Path, "/devtools/page/") {
			return "", fmt.Errorf("page-level CDP URLs are not supported; use a browser-level URL (ws://host:port/devtools/browser/<id>)")
		}
		if !strings.Contains(parsed.Path, "/devtools/browser/") {
			return "", fmt.Errorf("expected browser-level CDP path (/devtools/browser/<id>) in %q", trimmed)
		}
		pinned, err := validateAndResolveCDPHost(parsed, allowedHosts)
		if err != nil {
			return "", err
		}
		parsed.Host = urlHostForAddr(pinned, parsed.Port())
		return parsed.String(), nil
	case "http", "https":
		return resolveDevToolsURL(parsed, allowedHosts, allowedSchemes)
	default:
		return "", fmt.Errorf("unsupported cdpUrl scheme %q", parsed.Scheme)
	}
}

func resolveDevToolsURL(parsed *url.URL, allowedHosts, allowedSchemes []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe, pinned, err := probeCDPVersionParsed(ctx, parsed, allowedHosts, allowedSchemes)
	if err != nil {
		return "", err
	}

	return normalizeResolvedDevToolsURL(probe.WebSocketDebuggerURL, parsed, pinned, allowedHosts)
}

func ProbeCDPVersion(ctx context.Context, raw string, allowedHosts []string) (CDPVersionProbe, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return CDPVersionProbe{}, fmt.Errorf("cdpUrl is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return CDPVersionProbe{}, fmt.Errorf("invalid cdpUrl: %w", err)
	}
	probe, _, err := probeCDPVersionParsed(ctx, parsed, allowedHosts, defaultCDPAllowSchemes)
	return probe, err
}

func probeCDPVersionParsed(ctx context.Context, parsed *url.URL, allowedHosts, allowedSchemes []string) (CDPVersionProbe, netip.Addr, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if parsed.Host == "" {
		return CDPVersionProbe{}, netip.Addr{}, fmt.Errorf("cdpUrl must include a host")
	}
	if parsed.User != nil {
		return CDPVersionProbe{}, netip.Addr{}, fmt.Errorf("cdpUrl must not include credentials")
	}

	var versionURL string
	var httpScheme string
	scheme := strings.ToLower(parsed.Scheme)
	if err := validateCDPScheme(scheme, allowedSchemes); err != nil {
		return CDPVersionProbe{}, netip.Addr{}, err
	}
	switch scheme {
	case "ws", "wss":
		if strings.Contains(parsed.Path, "/devtools/page/") {
			return CDPVersionProbe{}, netip.Addr{}, fmt.Errorf("page-level CDP URLs are not supported; use a browser-level URL (ws://host:port/devtools/browser/<id>)")
		}
		if !strings.Contains(parsed.Path, "/devtools/browser/") {
			return CDPVersionProbe{}, netip.Addr{}, fmt.Errorf("expected browser-level CDP path (/devtools/browser/<id>) in %q", parsed.String())
		}
		httpScheme = "http"
		if strings.ToLower(parsed.Scheme) == "wss" {
			httpScheme = "https"
		}
		versionURL = (&url.URL{Scheme: httpScheme, Host: parsed.Host, Path: "/json/version"}).String()
	case "http", "https":
		if strings.Contains(parsed.Path, "/devtools/page/") {
			return CDPVersionProbe{}, netip.Addr{}, fmt.Errorf("page-level CDP URLs are not supported; use a browser-level URL or DevTools origin")
		}
		if parsed.Path != "" && parsed.Path != "/" && !strings.HasSuffix(parsed.Path, "/json/version") {
			return CDPVersionProbe{}, netip.Addr{}, fmt.Errorf("HTTP cdpUrl must be the DevTools origin or end with /json/version, got %q", parsed.Path)
		}
		httpScheme = strings.ToLower(parsed.Scheme)
		versionURL = (&url.URL{Scheme: httpScheme, Host: parsed.Host, Path: "/json/version"}).String()
	default:
		return CDPVersionProbe{}, netip.Addr{}, fmt.Errorf("unsupported cdpUrl scheme %q", parsed.Scheme)
	}

	pinned, err := validateAndResolveCDPHostWithContext(ctx, parsed, allowedHosts)
	if err != nil {
		return CDPVersionProbe{}, netip.Addr{}, err
	}
	probe, err := fetchCDPVersion(ctx, versionURL, pinned, portForScheme(httpScheme, parsed.Port()))
	if err != nil {
		return CDPVersionProbe{}, netip.Addr{}, err
	}
	return probe, pinned, nil
}

func fetchCDPVersion(ctx context.Context, target string, pinned netip.Addr, port string) (CDPVersionProbe, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return CDPVersionProbe{}, err
	}
	resp, err := pinnedCDPHTTPClient(pinned, port).Do(req)
	if err != nil {
		return CDPVersionProbe{}, fmt.Errorf("fetch %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return CDPVersionProbe{}, fmt.Errorf("fetch %s: HTTP %d", target, resp.StatusCode)
	}
	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	body, err := readCDPVersionBody(resp.Body, target)
	if err != nil {
		return CDPVersionProbe{}, err
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return CDPVersionProbe{}, fmt.Errorf("decode %s: %w", target, err)
	}
	if info.WebSocketDebuggerURL == "" {
		return CDPVersionProbe{}, fmt.Errorf("missing webSocketDebuggerUrl in %s", target)
	}
	return CDPVersionProbe{VersionURL: target, WebSocketDebuggerURL: info.WebSocketDebuggerURL}, nil
}

func normalizeResolvedDevToolsURL(raw string, requested *url.URL, requestedPinned netip.Addr, allowedHosts []string) (string, error) {
	wsURL, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid webSocketDebuggerUrl %q: %w", raw, err)
	}
	switch strings.ToLower(wsURL.Scheme) {
	case "ws", "wss":
	default:
		return "", fmt.Errorf("webSocketDebuggerUrl must use ws or wss, got %q", wsURL.Scheme)
	}
	if strings.Contains(wsURL.Path, "/devtools/page/") {
		return "", fmt.Errorf("page-level CDP URLs are not supported; use a browser-level URL")
	}
	if !strings.Contains(wsURL.Path, "/devtools/browser/") {
		return "", fmt.Errorf("expected browser-level CDP path (/devtools/browser/<id>) in webSocketDebuggerUrl")
	}
	if wsURL.User != nil {
		return "", fmt.Errorf("webSocketDebuggerUrl must not include credentials")
	}
	if wsURL.Host == "" {
		return "", fmt.Errorf("webSocketDebuggerUrl must include a host")
	}
	if requested != nil && requested.Host != "" && shouldUseRequestedDevToolsHost(wsURL.Hostname()) {
		wsURL.Host = urlHostForAddr(requestedPinned, requested.Port())
		return wsURL.String(), nil
	}

	if requestedPinned.IsValid() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		addrs, err := resolveCDPHostAddrs(ctx, wsURL.Hostname())
		cancel()
		if err == nil {
			for _, addr := range addrs {
				if addr == requestedPinned {
					wsURL.Host = urlHostForAddr(requestedPinned, wsURL.Port())
					return wsURL.String(), nil
				}
			}
		}
	}

	requestedHost := ""
	if requested != nil {
		requestedHost = requested.Hostname()
	}
	return "", fmt.Errorf("webSocketDebuggerUrl host %q does not match requested CDP endpoint %q", wsURL.Hostname(), requestedHost)
}

func shouldUseRequestedDevToolsHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

func validateAndResolveCDPHost(parsed *url.URL, allowedHosts []string) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return validateAndResolveCDPHostWithContext(ctx, parsed, allowedHosts)
}

func validateAndResolveCDPHostWithContext(ctx context.Context, parsed *url.URL, allowedHosts []string) (netip.Addr, error) {
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return netip.Addr{}, fmt.Errorf("cdpUrl must include a host")
	}

	addrs, err := resolveCDPHostAddrs(ctx, host)
	if err != nil {
		return netip.Addr{}, err
	}
	allowlisted := cdpHostAllowed(host, allowedHosts)
	allLoopback := true
	for _, addr := range addrs {
		if !addr.IsLoopback() {
			allLoopback = false
			break
		}
	}
	if !allLoopback && !allowlisted {
		return netip.Addr{}, fmt.Errorf("remote CDP host %q resolved to non-loopback address; add it to security.attach.allowHosts to allow remote attach", host)
	}
	if !allLoopback {
		slog.Warn("remote CDP endpoint is non-loopback; CDP attach grants full browser control", "host", host, "scheme", parsed.Scheme)
	}
	return addrs[0], nil
}

func resolveCDPHostAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip := net.ParseIP(host); ip != nil {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, fmt.Errorf("invalid cdpUrl host %q", host)
		}
		return []netip.Addr{addr.Unmap()}, nil
	}
	ips, err := cdpResolveHostIPs(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve remote CDP host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve remote CDP host %q: no addresses", host)
	}
	out := make([]netip.Addr, 0, len(ips))
	seen := make(map[netip.Addr]struct{}, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("resolve remote CDP host %q: no usable IPs", host)
	}
	return out, nil
}

func cdpHostAllowed(host string, allowedHosts []string) bool {
	host = strings.ToLower(strings.Trim(host, "[] "))
	for _, raw := range allowedHosts {
		allowed := strings.ToLower(strings.Trim(raw, "[] "))
		if allowed == "*" || allowed == host {
			return true
		}
	}
	return false
}

func validateCDPScheme(scheme string, allowedSchemes []string) error {
	switch scheme {
	case "ws", "wss", "http", "https":
	default:
		return fmt.Errorf("unsupported cdpUrl scheme %q", scheme)
	}

	effective := effectiveCDPAllowSchemes(allowedSchemes)
	for _, allowed := range effective {
		if scheme == allowed {
			return nil
		}
	}
	return fmt.Errorf("remote CDP scheme %q not allowed (allowed: %v)", scheme, effective)
}

func effectiveCDPAllowSchemes(allowedSchemes []string) []string {
	if len(allowedSchemes) == 0 {
		return append([]string(nil), defaultCDPAllowSchemes...)
	}
	out := make([]string, 0, len(allowedSchemes))
	for _, raw := range allowedSchemes {
		scheme := strings.ToLower(strings.TrimSpace(raw))
		if scheme != "" {
			out = append(out, scheme)
		}
	}
	return out
}

func pinnedCDPHTTPClient(addr netip.Addr, port string) *http.Client {
	return &http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: 3 * time.Second}
				return dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			},
			TLSHandshakeTimeout: 3 * time.Second,
		},
	}
}

func portForScheme(scheme, explicit string) string {
	if explicit != "" {
		return explicit
	}
	switch strings.ToLower(scheme) {
	case "https", "wss":
		return "443"
	default:
		return "80"
	}
}

func readCDPVersionBody(r io.Reader, target string) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, cdpVersionBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", target, err)
	}
	if len(body) > cdpVersionBodyLimit {
		return nil, fmt.Errorf("read %s: response body exceeds %d bytes", target, cdpVersionBodyLimit)
	}
	return body, nil
}

func urlHostForAddr(addr netip.Addr, port string) string {
	host := addr.String()
	if port == "" {
		if addr.Is6() {
			return "[" + host + "]"
		}
		return host
	}
	return net.JoinHostPort(host, port)
}
