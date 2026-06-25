package navguard

import (
	"net"
	"net/netip"
)

// Validator carries pre-parsed security configuration for navigation target
// validation. Handlers construct one per call because trusted CIDRs come from
// the per-request (target-scoped) effective config; remote-IP validation uses
// the standalone ValidateRemoteIP function. It is safe for concurrent use when
// fields are not mutated after construction.
type Validator struct {
	TrustedResolveCIDRs []*net.IPNet
}

// ValidatedTarget is the result of ValidateTarget. It signals which runtime
// checks may be relaxed for this navigation.
type ValidatedTarget struct {
	AllowInternal     bool
	TrustedResolvedIP []netip.Addr
}
