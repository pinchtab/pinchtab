package navguard

import (
	"net"
	"net/netip"
)

// Validator carries pre-parsed security configuration for navigation URL, target,
// and remote-IP validation. It is safe for concurrent use when fields are not mutated
// after construction.
type Validator struct {
	TrustedResolveCIDRs []*net.IPNet
	TrustedProxyCIDRs   []*net.IPNet
	IDPIDomainAllowed   func(rawURL string) bool // nil = no IDPI check
}

// ValidatedTarget is the result of ValidateTarget. It signals which runtime
// checks may be relaxed for this navigation.
type ValidatedTarget struct {
	AllowInternal     bool
	TrustedResolvedIP []netip.Addr
}
