package scheduler

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var resolveWebhookHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, network, host)
}

var blockedWebhookPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("198.18.0.0/15"),
}

type callbackURLGuard struct{}

func newCallbackURLGuard() *callbackURLGuard { return &callbackURLGuard{} }

type validatedCallbackTarget struct {
	URL  *url.URL
	Host string
	Port string
	IPs  []netip.Addr
}

func (g *callbackURLGuard) Validate(rawURL string) error {
	_, err := g.ValidateTarget(rawURL)
	return err
}

func (g *callbackURLGuard) ValidateTarget(rawURL string) (*validatedCallbackTarget, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid callback URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("callback URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("callback URL host is required")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("callback URL credentials are not allowed")
	}

	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("callback URL host is not allowed")
	}
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return nil, fmt.Errorf("callback URL must use http or https")
		}
	}

	target := &validatedCallbackTarget{
		URL:  parsed,
		Host: host,
		Port: port,
	}

	if ip := net.ParseIP(host); ip != nil {
		addr, err := validateWebhookIP(ip)
		if err != nil {
			return nil, err
		}
		target.IPs = []netip.Addr{addr}
		return target, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ips, err := resolveWebhookHostIPs(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("could not resolve callback host")
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("could not resolve callback host")
	}
	seen := make(map[netip.Addr]struct{}, len(ips))
	for _, ip := range ips {
		addr, err := validateWebhookIP(ip)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		target.IPs = append(target.IPs, addr)
	}
	if len(target.IPs) == 0 {
		return nil, fmt.Errorf("could not resolve callback host")
	}
	return target, nil
}

func validateCallbackURL(rawURL string) error {
	return newCallbackURLGuard().Validate(rawURL)
}

func validateCallbackTarget(rawURL string) (*validatedCallbackTarget, error) {
	return newCallbackURLGuard().ValidateTarget(rawURL)
}

func validateWebhookIP(ip net.IP) (netip.Addr, error) {
	if ip == nil {
		return netip.Addr{}, fmt.Errorf("callback URL host is not allowed")
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, fmt.Errorf("callback URL host is not allowed")
	}
	addr = addr.Unmap()
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return netip.Addr{}, fmt.Errorf("callback URL host is not allowed")
	}
	for _, prefix := range blockedWebhookPrefixes {
		if prefix.Contains(addr) {
			return netip.Addr{}, fmt.Errorf("callback URL host is not allowed")
		}
	}
	return addr, nil
}
