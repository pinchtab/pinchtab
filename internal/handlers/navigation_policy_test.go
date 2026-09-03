package handlers

import (
	"net"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/navguard"
)

func TestBuildNavigateTrustedProxyCIDRs_NilConfig(t *testing.T) {
	if got := buildNavigateTrustedProxyCIDRs(nil); got != nil {
		t.Fatalf("buildNavigateTrustedProxyCIDRs(nil) = %v, want nil", got)
	}
}

func TestBuildNavigateTrustedProxyCIDRs_FlagOff(t *testing.T) {
	cfg := &config.RuntimeConfig{
		TrustedProxyCIDRs:  []string{"10.0.0.0/8"},
		TrustLoopbackProxy: false,
	}
	got := buildNavigateTrustedProxyCIDRs(cfg)
	if len(got) != 1 {
		t.Fatalf("got %d CIDRs, want 1 (only the configured one); got=%v", len(got), got)
	}
	if got[0].String() != "10.0.0.0/8" {
		t.Errorf("got[0] = %s, want 10.0.0.0/8", got[0].String())
	}
	// Sanity: loopback must NOT be in the list when flag is off.
	if cidrsContainIP(got, net.ParseIP("127.0.0.1")) {
		t.Errorf("loopback 127.0.0.1 must not be trusted when TrustLoopbackProxy=false")
	}
}

func TestBuildNavigateTrustedProxyCIDRs_FlagOn(t *testing.T) {
	cfg := &config.RuntimeConfig{
		TrustedProxyCIDRs:  []string{"10.0.0.0/8"},
		TrustLoopbackProxy: true,
	}
	got := buildNavigateTrustedProxyCIDRs(cfg)
	if !cidrsContainIP(got, net.ParseIP("127.0.0.1")) {
		t.Errorf("127.0.0.1 must be trusted when TrustLoopbackProxy=true; got=%v", cidrStrings(got))
	}
	if !cidrsContainIP(got, net.ParseIP("::1")) {
		t.Errorf("::1 must be trusted when TrustLoopbackProxy=true; got=%v", cidrStrings(got))
	}
	// Existing configured CIDRs must still be present.
	if !cidrsContainIP(got, net.ParseIP("10.4.5.6")) {
		t.Errorf("configured CIDR 10.0.0.0/8 must remain trusted; got=%v", cidrStrings(got))
	}
	// Public IP must NOT be trusted.
	if cidrsContainIP(got, net.ParseIP("8.8.8.8")) {
		t.Errorf("public IP 8.8.8.8 must not be trusted; got=%v", cidrStrings(got))
	}
}

func TestValidateNavigateRemoteIPAddress_LoopbackProxyAllowed(t *testing.T) {
	cfg := &config.RuntimeConfig{TrustLoopbackProxy: true}
	trusted := buildNavigateTrustedProxyCIDRs(cfg)

	for _, ip := range []string{"127.0.0.1", "127.255.255.254", "::1"} {
		if err := navguard.ValidateRemoteIP(ip, trusted, nil); err != nil {
			t.Errorf("navguard.ValidateRemoteIP(%q) with TrustLoopbackProxy=true returned %v, want nil", ip, err)
		}
	}
}

func TestValidateNavigateRemoteIPAddress_LoopbackProxyBlockedByDefault(t *testing.T) {
	cfg := &config.RuntimeConfig{TrustLoopbackProxy: false}
	trusted := buildNavigateTrustedProxyCIDRs(cfg)

	for _, ip := range []string{"127.0.0.1", "::1"} {
		if err := navguard.ValidateRemoteIP(ip, trusted, nil); err == nil {
			t.Errorf("navguard.ValidateRemoteIP(%q) with TrustLoopbackProxy=false returned nil, want SSRF block", ip)
		}
	}
}

func cidrsContainIP(cidrs []*net.IPNet, ip net.IP) bool {
	for _, c := range cidrs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

func cidrStrings(cidrs []*net.IPNet) []string {
	out := make([]string, len(cidrs))
	for i, c := range cidrs {
		out[i] = c.String()
	}
	return out
}
