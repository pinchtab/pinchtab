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

func NormalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
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
	return nil
}

func ResolveAndValidatePublicIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	host = NormalizeHost(host)
	if host == "" {
		return nil, ErrResolveHost
	}

	if ip := net.ParseIP(host); ip != nil {
		addr, err := publicAddr(ip)
		if err != nil {
			return nil, err
		}
		return []netip.Addr{addr}, nil
	}

	ips, err := ResolveHostIPs(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrResolveHost
	}

	seen := make(map[netip.Addr]struct{}, len(ips))
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, err := publicAddr(ip)
		if err != nil {
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
		return nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return fmt.Errorf("%w %q", ErrUnparseableRemoteIP, raw)
	}
	return ValidatePublicIP(ip)
}

func publicAddr(ip net.IP) (netip.Addr, error) {
	if err := ValidatePublicIP(ip); err != nil {
		return netip.Addr{}, err
	}
	addr, _ := netip.AddrFromSlice(ip)
	return addr.Unmap(), nil
}
