package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

const liteDefaultMaxRedirects = 10

var dialLiteAddress = func(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, addr)
}

func (l *LiteEngine) clientForNavigate(ctx context.Context) *http.Client {
	policy := navigateNetworkPolicyFromContext(ctx)
	if policy == nil {
		return l.client
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialGuardedLiteAddress(ctx, network, addr, policy)
	}

	return &http.Client{
		Timeout:   l.client.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if exceededLiteRedirectLimit(len(via), policy.MaxRedirects) {
				return fmt.Errorf("too many redirects")
			}
			return validateLiteRedirectTarget(req, policy)
		},
	}
}

func exceededLiteRedirectLimit(hops, maxRedirects int) bool {
	if maxRedirects < 0 {
		maxRedirects = liteDefaultMaxRedirects
	}
	return hops > maxRedirects
}

func validateLiteRedirectTarget(req *http.Request, policy *NavigateNetworkPolicy) error {
	if req == nil || req.URL == nil || policy == nil || policy.AllowInternal {
		return nil
	}

	host := req.URL.Hostname()
	if host == "" {
		return fmt.Errorf("redirect target missing host")
	}
	if netguard.IsLocalHost(host) {
		return &NetworkPolicyBlockedError{Reason: fmt.Sprintf("redirect to blocked local host %s", host)}
	}
	if _, err := netguard.ResolveAndValidatePublicIPs(req.Context(), host); err != nil {
		if errors.Is(err, netguard.ErrPrivateInternalIP) {
			return &NetworkPolicyBlockedError{Reason: fmt.Sprintf("redirect to blocked private/internal host %s", host)}
		}
		return fmt.Errorf("redirect resolve %s: %w", host, err)
	}
	return nil
}

func dialGuardedLiteAddress(ctx context.Context, network, addr string, policy *NavigateNetworkPolicy) (net.Conn, error) {
	if policy == nil || policy.AllowInternal {
		return dialLiteAddress(ctx, network, addr)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := resolveLiteDialIPs(ctx, host, policy)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, ip := range ips {
		conn, err := dialLiteAddress(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no validated addresses for %s", host)
	}
	return nil, lastErr
}

func resolveLiteDialIPs(ctx context.Context, host string, policy *NavigateNetworkPolicy) ([]net.IP, error) {
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := validateLiteDialIP(ip, policy); err != nil {
			return nil, err
		}
		return []net.IP{ip}, nil
	}

	ips, err := netguard.ResolveHostIPs(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, netguard.ErrResolveHost
	}

	seen := make(map[netip.Addr]struct{}, len(ips))
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if err := validateLiteDialIP(ip, policy); err != nil {
			return nil, err
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, &NetworkPolicyBlockedError{Reason: fmt.Sprintf("navigation connected to blocked remote IP %s", ip.String())}
		}
		addr = addr.Unmap()
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, ip)
	}
	return out, nil
}

func validateLiteDialIP(ip net.IP, policy *NavigateNetworkPolicy) error {
	if policy == nil || policy.AllowInternal {
		return nil
	}
	if err := netguard.ValidatePublicIP(ip); err == nil {
		return nil
	}

	if ipInTrustedCIDRs(ip, policy.TrustedProxyCIDRs) {
		return nil
	}

	if addr, ok := netip.AddrFromSlice(ip); ok {
		addr = addr.Unmap()
		for _, trusted := range policy.TrustedResolvedIP {
			if trusted == addr {
				return nil
			}
		}
	}

	return &NetworkPolicyBlockedError{Reason: fmt.Sprintf("navigation connected to blocked remote IP %s", ip.String())}
}

func ipInTrustedCIDRs(ip net.IP, trusted []*net.IPNet) bool {
	for _, cidr := range trusted {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
