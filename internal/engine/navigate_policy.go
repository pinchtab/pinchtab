package engine

import (
	"context"
	"errors"
	"net"
	"net/netip"
)

// NavigateNetworkPolicy carries per-request network validation settings for
// engines that perform server-side HTTP fetches.
type NavigateNetworkPolicy struct {
	AllowInternal     bool
	TrustedProxyCIDRs []*net.IPNet
	TrustedResolvedIP []netip.Addr
	MaxRedirects      int
}

type navigateNetworkPolicyKey struct{}

// WithNavigateNetworkPolicy attaches navigation network policy to a context.
func WithNavigateNetworkPolicy(ctx context.Context, policy *NavigateNetworkPolicy) context.Context {
	if policy == nil {
		return ctx
	}
	clone := &NavigateNetworkPolicy{
		AllowInternal:     policy.AllowInternal,
		TrustedProxyCIDRs: append([]*net.IPNet(nil), policy.TrustedProxyCIDRs...),
		TrustedResolvedIP: append([]netip.Addr(nil), policy.TrustedResolvedIP...),
		MaxRedirects:      policy.MaxRedirects,
	}
	return context.WithValue(ctx, navigateNetworkPolicyKey{}, clone)
}

func navigateNetworkPolicyFromContext(ctx context.Context) *NavigateNetworkPolicy {
	policy, _ := ctx.Value(navigateNetworkPolicyKey{}).(*NavigateNetworkPolicy)
	return policy
}

// NetworkPolicyBlockedError reports a navigation blocked by SSRF protections.
type NetworkPolicyBlockedError struct {
	Reason string
}

func (e *NetworkPolicyBlockedError) Error() string { return e.Reason }

// IsNetworkPolicyBlocked reports whether err is a network policy block.
func IsNetworkPolicyBlocked(err error) bool {
	var target *NetworkPolicyBlockedError
	return errors.As(err, &target)
}
