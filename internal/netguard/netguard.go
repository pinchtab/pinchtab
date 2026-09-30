package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
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

func NormalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
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
	host = NormalizeHost(host)
	if host == "" {
		return nil, ErrResolveHost
	}

	if ip := net.ParseIP(host); ip != nil {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, ErrPrivateInternalIP
		}
		return ValidateResolvedPublicAddrs([]netip.Addr{addr})
	}

	ips, err := ResolveHostIPs(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrResolveHost
	}

	addrs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, ErrPrivateInternalIP
		}
		addrs = append(addrs, addr)
	}
	return ValidateResolvedPublicAddrs(addrs)
}

func ValidateResolvedPublicAddrs(ips []netip.Addr) ([]netip.Addr, error) {
	if len(ips) == 0 {
		return nil, ErrResolveHost
	}
	seen := make(map[netip.Addr]struct{}, len(ips))
	out := make([]netip.Addr, 0, len(ips))
	for _, addr := range ips {
		if !addr.IsValid() {
			return nil, ErrPrivateInternalIP
		}
		addr = addr.Unmap()
		if err := ValidatePublicIP(net.IP(addr.AsSlice())); err != nil {
			return nil, err
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	if len(out) == 0 {
		return nil, ErrResolveHost
	}
	return out, nil
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
	host = NormalizeHost(host)
	if host == "" {
		return nil, ErrResolveHost
	}

	if ip := net.ParseIP(host); ip != nil {
		if err := ValidatePublicIP(ip); err != nil {
			if !ipInCIDRs(ip, trusted) {
				return nil, err
			}
		}
		addr, _ := netip.AddrFromSlice(ip)
		return []netip.Addr{addr.Unmap()}, nil
	}

	ips, err := ResolveHostIPs(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrResolveHost
	}

	seen := make(map[netip.Addr]struct{}, len(ips))
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if err := ValidatePublicIP(ip); err != nil {
			if !ipInCIDRs(ip, trusted) {
				return nil, err
			}
		}
		addr, _ := netip.AddrFromSlice(ip)
		addr = addr.Unmap()
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	if len(out) == 0 {
		return nil, ErrResolveHost
	}
	return out, nil
}

func ipInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
