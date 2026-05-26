package navguard

import (
	"fmt"
	"net"
	"net/netip"
	"slices"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

// ValidateRemoteIP checks that the response RemoteIPAddress is a public IP or
// falls within trusted CIDRs / pre-resolved IPs. It uses the Validator's
// TrustedProxyCIDRs.
func (v *Validator) ValidateRemoteIP(raw string, trustedResolved []netip.Addr) error {
	return ValidateRemoteIP(raw, v.TrustedProxyCIDRs, trustedResolved)
}

// ValidateRemoteIP is the standalone (non-method) remote IP validator.
func ValidateRemoteIP(raw string, trustedCIDRs []*net.IPNet, trustedIPs []netip.Addr) error {
	normalized := netguard.NormalizeRemoteIP(raw)
	if err := netguard.ValidateRemoteIPAddress(raw); err != nil {
		if ip := net.ParseIP(normalized); ip != nil {
			for _, cidr := range trustedCIDRs {
				if cidr.Contains(ip) {
					return nil
				}
			}
			if addr, ok := netip.AddrFromSlice(ip); ok {
				addr = addr.Unmap()
				if slices.Contains(trustedIPs, addr) {
					return nil
				}
			}
		}
		return fmt.Errorf("navigation connected to blocked remote IP %s", normalized)
	}
	return nil
}
