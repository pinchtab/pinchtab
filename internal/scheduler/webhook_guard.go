package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

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

	host := netguard.NormalizeHost(parsed.Hostname())
	if host == "" || netguard.IsLocalHost(host) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ips, err := netguard.ResolveAndValidatePublicIPs(ctx, host)
	if err != nil {
		if errors.Is(err, netguard.ErrResolveHost) {
			return nil, fmt.Errorf("could not resolve callback host")
		}
		if errors.Is(err, netguard.ErrPrivateInternalIP) {
			return nil, fmt.Errorf("callback URL host is not allowed")
		}
		return nil, fmt.Errorf("could not resolve callback host")
	}
	target.IPs = append(target.IPs, ips...)
	return target, nil
}

func validateCallbackURL(rawURL string) error {
	return newCallbackURLGuard().Validate(rawURL)
}

func validateCallbackTarget(rawURL string) (*validatedCallbackTarget, error) {
	return newCallbackURLGuard().ValidateTarget(rawURL)
}
