package navguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

// ValidateTarget resolves a navigation URL's target host and enforces SSRF
// protections using the Validator's TrustedResolveCIDRs. IDPI domain
// allowlisting happens upstream: callers compute allowExplicitInternal from
// the IDPI allowlist (handlers derive it via idpi Guard.DomainAllowed) and it
// overrides the private-IP block for explicitly allowed domains.
func (v *Validator) ValidateTarget(ctx context.Context, raw string, allowExplicitInternal bool) (*ValidatedTarget, error) {
	return ValidateTarget(ctx, raw, allowExplicitInternal, v.TrustedResolveCIDRs)
}

// ValidateTarget is the standalone target validator usable without a Validator instance.
func ValidateTarget(ctx context.Context, raw string, allowExplicitInternal bool, trustedResolveCIDRs []*net.IPNet) (*ValidatedTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "about:blank") {
		return &ValidatedTarget{AllowInternal: true}, nil
	}

	host, hasHost := ExtractHost(raw)
	if !hasHost {
		// No resolvable host means the IP/SSRF checks below can't run, so a
		// caller that relies on ValidateTarget alone would otherwise allow any
		// scheme. Enforce scheme safety here too (defense in depth) so opaque
		// targets like file:/data: are rejected regardless of caller ordering;
		// scheme-less/relative inputs and about:blank still pass ValidateURL.
		if err := ValidateURL(raw); err != nil {
			return nil, err
		}
		return &ValidatedTarget{}, nil
	}
	if netguard.IsLocalHost(host) {
		return &ValidatedTarget{AllowInternal: true}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	ips, err := ResolveHostAddrs(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("could not resolve navigation host")
	}
	if err := validateResolvedPublicIPs(ips); err != nil {
		if errors.Is(err, netguard.ErrPrivateInternalIP) {
			if allowExplicitInternal {
				return &ValidatedTarget{TrustedResolvedIP: ips}, nil
			}
			if len(trustedResolveCIDRs) > 0 && validateResolvedIPsWithTrustedCIDRs(ips, trustedResolveCIDRs) == nil {
				cidrs := make([]string, len(trustedResolveCIDRs))
				for i, c := range trustedResolveCIDRs {
					cidrs[i] = c.String()
				}
				addrs := make([]string, len(ips))
				for i, a := range ips {
					addrs[i] = a.String()
				}
				slog.Info("navigate: trusted resolve CIDR override",
					"host", host,
					"resolvedIPs", addrs,
					"trustedCIDRs", cidrs,
				)
				return &ValidatedTarget{TrustedResolvedIP: ips}, nil
			}
			return nil, fmt.Errorf("navigation target resolves to blocked private/internal IP")
		}
		return nil, fmt.Errorf("could not resolve navigation host")
	}
	return &ValidatedTarget{}, nil
}

func validateResolvedPublicIPs(ips []netip.Addr) error {
	_, err := netguard.ValidateResolvedPublicAddrs(ips)
	return err
}

func validateResolvedIPsWithTrustedCIDRs(ips []netip.Addr, trusted []*net.IPNet) error {
	if len(ips) == 0 {
		return netguard.ErrResolveHost
	}
	for _, addr := range ips {
		ip := net.ParseIP(addr.String())
		if err := netguard.ValidatePublicIP(ip); err != nil {
			if !IPInCIDRs(ip, trusted) {
				return err
			}
		}
	}
	return nil
}

// ResolveHostAddrs resolves a host to a deduplicated set of IP addresses.
func ResolveHostAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	host = netguard.NormalizeHost(host)
	if host == "" {
		return nil, netguard.ErrResolveHost
	}
	if ip := net.ParseIP(host); ip != nil {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, netguard.ErrResolveHost
		}
		return []netip.Addr{addr.Unmap()}, nil
	}

	ips, err := netguard.ResolveHostIPs(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, netguard.ErrResolveHost
	}
	seen := make(map[netip.Addr]struct{}, len(ips))
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, netguard.ErrResolveHost
		}
		addr = addr.Unmap()
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	if len(out) == 0 {
		return nil, netguard.ErrResolveHost
	}
	return out, nil
}

// IPInCIDRs reports whether ip falls within any of the given CIDRs.
func IPInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
