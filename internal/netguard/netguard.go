package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/urls"
)

var (
	ErrResolveHost         = errors.New("could not resolve host")
	ErrPrivateInternalIP   = errors.New("private/internal IP blocked")
	ErrUnparseableRemoteIP = errors.New("unparseable remote IP")
)

var ResolveHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, network, host)
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("198.18.0.0/15"),
}

var nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")

// embeddedIPv4 extracts the IPv4 address carried inside an IPv6 transition
// address (6to4 2002::/16, NAT64 64:ff9b::/96). These wrappers are globally
// scoped, so IsPrivate/IsLoopback/IsLinkLocalUnicast all report false while the
// traffic still reaches the embedded IPv4 — 64:ff9b::a9fe:a9fe is the cloud
// metadata service. The embedded address is decoded rather than the prefixes
// being banned outright so transition addresses wrapping a public IPv4 keep
// working.
func embeddedIPv4(addr netip.Addr) (netip.Addr, bool) {
	if !addr.Is6() {
		return netip.Addr{}, false
	}
	b := addr.As16()
	if b[0] == 0x20 && b[1] == 0x02 {
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), true
	}
	if nat64WellKnownPrefix.Contains(addr) {
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
	}
	return netip.Addr{}, false
}

// NormalizeHost lowercases host and drops the DNS root label: "example.com." and
// "example.com" name the same host and the browser fetches both. The allowlist is
// read in both directions, so a listed host that stopped matching would refuse a
// target the operator allowed and, read inverted by the response-forgery rule,
// permit forgery on exactly the sensitive origin the list marks.
func NormalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// ExtractHost returns the normalized host rawURL names; urls.EnsureScheme owns the
// normalisation that decides which forms carry a host.
func ExtractHost(rawURL string) string {
	parsed, err := url.Parse(urls.EnsureScheme(rawURL))
	if err != nil {
		return ""
	}
	return NormalizeHost(parsed.Hostname())
}

// HostAllowed reports whether rawURL's host matches an entry in allowedDomains.
//
// An empty allowedDomains list means "no allowlist active" → returns true.
// A non-empty list with no matching host → returns false. Special non-routable
// URLs ("about:blank") are always allowed because they have no host to verify.
//
// Supported pattern forms:
//   - "example.com"   – exact host match (case-insensitive, port stripped)
//   - "*.example.com" – any single subdomain of example.com but NOT example.com
//   - "*"             – matches any host (effectively disables the allowlist)
func HostAllowed(rawURL string, allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return true
	}
	if IsAllowedSpecialURL(rawURL) {
		return true
	}
	host := ExtractHost(rawURL)
	if host == "" {
		return false
	}
	return HostMatchesPatterns(host, allowedDomains)
}

// HostMatchesPatterns reports whether host (already normalized) matches any of
// the patterns, for callers that already have a host.
func HostMatchesPatterns(host string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if matchDomain(host, pattern) {
			return true
		}
	}
	return false
}

// IsAllowedSpecialURL reports whether rawURL is a non-routable URL that bypasses
// host-based allowlist checks (e.g. "about:blank").
func IsAllowedSpecialURL(rawURL string) bool {
	return strings.EqualFold(strings.TrimSpace(rawURL), "about:blank")
}

func matchDomain(host, pattern string) bool {
	switch {
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*."):
		return strings.HasSuffix(host, pattern[1:])
	default:
		return host == pattern
	}
}

func HostInAllowlist(host string, allowlist []string) bool {
	host = normalizeAllowlistHost(host)
	for _, raw := range allowlist {
		allowed := normalizeAllowlistHost(raw)
		if allowed == "*" || (allowed != "" && allowed == host) {
			return true
		}
	}
	return false
}

func normalizeAllowlistHost(host string) string {
	return NormalizeHost(strings.Trim(strings.TrimSpace(host), "[]"))
}

func AttachSchemeAllowed(scheme string, allowlist []string) bool {
	scheme = strings.TrimSpace(scheme)
	for _, allowed := range allowlist {
		if scheme != "" && strings.EqualFold(scheme, strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func IsLocalHost(host string) bool {
	host = NormalizeHost(host)
	if host == "" {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return addr.Unmap().IsLoopback()
}

func ValidatePublicIP(ip net.IP) error {
	if ip == nil {
		return ErrPrivateInternalIP
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return ErrPrivateInternalIP
	}
	addr = addr.Unmap()
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return ErrPrivateInternalIP
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return ErrPrivateInternalIP
		}
	}
	if v4, ok := embeddedIPv4(addr); ok {
		return ValidatePublicIP(net.IP(v4.AsSlice()))
	}
	return nil
}

func ResolveAndValidatePublicIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	return ResolveAndValidateIPsWithTrustedCIDRs(ctx, host, nil)
}

// ValidateAddrs refuses the first address that is not public unless a trusted CIDR
// covers it; an empty set is a resolution failure, never a pass.
func ValidateAddrs(addrs []netip.Addr, trusted []*net.IPNet) error {
	if len(addrs) == 0 {
		return ErrResolveHost
	}
	for _, addr := range addrs {
		ip := net.IP(addr.Unmap().AsSlice())
		if err := ValidatePublicIP(ip); err != nil && !IPInCIDRs(ip, trusted) {
			return err
		}
	}
	return nil
}

func NormalizeRemoteIP(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	return raw
}

func ValidateRemoteIPAddress(raw string) error {
	raw = NormalizeRemoteIP(raw)
	if raw == "" {
		// Accepted by design: CDP reports no remote IP for cache- and
		// service-worker-served responses, so erroring here would break
		// legitimate cached navigations. This post-connect check is the last
		// of several layers (URL validation, resolve-time checks with pinned
		// IPs, dial-time enforcement on the static path); the residual
		// exposure is a cached/SW response for an already-validated target.
		return nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return fmt.Errorf("%w %q", ErrUnparseableRemoteIP, raw)
	}
	return ValidatePublicIP(ip)
}

func ResolveAndValidateIPsWithTrustedCIDRs(ctx context.Context, host string, trusted []*net.IPNet) ([]netip.Addr, error) {
	addrs, err := ResolveHostAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	if err := ValidateAddrs(addrs, trusted); err != nil {
		return nil, err
	}
	return addrs, nil
}

func ResolveHostAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	return ResolveHostAddrsWith(ctx, host, ResolveHostIPs)
}

// ResolveHostAddrsWith resolves host through resolve, or takes it as a literal IP, and
// returns its addresses unmapped and deduplicated in resolver order.
func ResolveHostAddrsWith(ctx context.Context, host string, resolve func(ctx context.Context, network, host string) ([]net.IP, error)) ([]netip.Addr, error) {
	host = NormalizeHost(host)
	if host == "" {
		return nil, ErrResolveHost
	}
	ips := []net.IP{net.ParseIP(host)}
	if ips[0] == nil {
		resolved, err := resolve(ctx, "ip", host)
		if err != nil || len(resolved) == 0 {
			return nil, ErrResolveHost
		}
		ips = resolved
	}
	seen := make(map[netip.Addr]struct{}, len(ips))
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, ErrResolveHost
		}
		addr = addr.Unmap()
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out, nil
}

func IPInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
