package navguard

import (
	"net"
	"strings"
)

// loopbackProxyCIDRs are the CIDRs implicitly trusted as legitimate proxy hops
// when security.trustLoopbackProxy is enabled. Covers IPv4 and IPv6 loopback.
var loopbackProxyCIDRs = func() []*net.IPNet {
	cidrs := make([]*net.IPNet, 0, 2)
	for _, s := range []string{"127.0.0.0/8", "::1/128"} {
		if _, n, err := net.ParseCIDR(s); err == nil {
			cidrs = append(cidrs, n)
		}
	}
	return cidrs
}()

// BuildTrustedProxyCIDRs returns the list of trusted CIDRs used by the runtime
// navigation guard when checking the response RemoteIPAddress. It merges
// configuredCIDRs with the implicit loopback list when trustLoopback is enabled,
// giving users a one-flag escape from the SSRF guard tripping on a local
// HTTP/SOCKS proxy hop (e.g. macOS system proxy on 127.0.0.1) without having to
// hand-craft a CIDR list.
func BuildTrustedProxyCIDRs(trustLoopback bool, configuredCIDRs []string) []*net.IPNet {
	trusted := ParseCIDRs(configuredCIDRs)
	if trustLoopback {
		trusted = append(trusted, loopbackProxyCIDRs...)
	}
	return trusted
}

// ParseCIDRs parses a list of CIDR notation strings and bare IPs into IPNets.
// Bare IPs are treated as /32 (IPv4) or /128 (IPv6). Empty/blank entries are
// skipped; unparseable entries are silently dropped.
func ParseCIDRs(raw []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil {
				if ip.To4() != nil {
					s = ip.String() + "/32"
				} else {
					s = ip.String() + "/128"
				}
			} else {
				s += "/32"
			}
		}
		if _, cidr, err := net.ParseCIDR(s); err == nil {
			nets = append(nets, cidr)
		}
	}
	return nets
}
