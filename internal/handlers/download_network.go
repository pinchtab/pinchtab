package handlers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

const downloadFallbackDefaultMaxRedirects = 10

var dialDownloadAddress = func(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, addr)
}

func newGuardedDownloadClient(validator *downloadURLGuard, maxRedirects int, timeoutSeconds int) *http.Client {
	if validator == nil {
		validator = newDownloadURLGuard(nil)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialGuardedDownloadAddress(ctx, network, addr, validator)
	}

	return &http.Client{
		Timeout:   timeDurationSeconds(timeoutSeconds),
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if exceededDownloadRedirectLimit(len(via), maxRedirects) {
				return fmt.Errorf("too many redirects")
			}
			if err := validator.Validate(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

func exceededDownloadRedirectLimit(hops, maxRedirects int) bool {
	if maxRedirects < 0 {
		maxRedirects = downloadFallbackDefaultMaxRedirects
	}
	return hops > maxRedirects
}

func dialGuardedDownloadAddress(ctx context.Context, network, addr string, validator *downloadURLGuard) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	allowInternal := validator != nil && validator.isHostAllowed(host)
	ips, err := resolveDownloadDialIPs(ctx, host, allowInternal)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, ip := range ips {
		conn, err := dialDownloadAddress(ctx, network, net.JoinHostPort(ip.String(), port))
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

func resolveDownloadDialIPs(ctx context.Context, host string, allowInternal bool) ([]net.IP, error) {
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !allowInternal {
			if err := validateDownloadDialIP(ip); err != nil {
				return nil, err
			}
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
		if !allowInternal {
			if err := validateDownloadDialIP(ip); err != nil {
				return nil, err
			}
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, fmt.Errorf("download connected to blocked remote IP %s", ip.String())
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

func validateDownloadDialIP(ip net.IP) error {
	if err := netguard.ValidatePublicIP(ip); err != nil {
		return fmt.Errorf("download connected to blocked remote IP %s", ip.String())
	}
	return nil
}

func timeDurationSeconds(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
