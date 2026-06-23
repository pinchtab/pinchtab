package navguard

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/netguard"
)

func stubHostResolution(t *testing.T, fn func(context.Context, string, string) ([]net.IP, error)) {
	t.Helper()
	old := netguard.ResolveHostIPs
	netguard.ResolveHostIPs = fn
	t.Cleanup(func() {
		netguard.ResolveHostIPs = old
	})
}

func TestValidateURL_RejectsUnsupportedSchemes(t *testing.T) {
	for _, rawURL := range []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"chrome://settings",
		"data:text/html,hello",
	} {
		if err := ValidateURL(rawURL); err == nil {
			t.Fatalf("ValidateURL(%q) should reject unsupported schemes", rawURL)
		}
	}
}

func TestValidateURL_AllowsHTTPHTTPSAndBareHostnames(t *testing.T) {
	for _, rawURL := range []string{
		"https://pinchtab.com",
		"http://pinchtab.test",
		"pinchtab.com",
		"about:blank",
	} {
		if err := ValidateURL(rawURL); err != nil {
			t.Fatalf("ValidateURL(%q) error = %v", rawURL, err)
		}
	}
}

func TestValidateURL_RejectsOverlongURL(t *testing.T) {
	rawURL := "https://pinchtab.com/" + strings.Repeat("a", MaxURLLen)
	if err := ValidateURL(rawURL); err == nil {
		t.Fatal("ValidateURL should reject overlong urls")
	}
}

func TestValidateURL_RejectsEmpty(t *testing.T) {
	if err := ValidateURL(""); err == nil {
		t.Fatal("ValidateURL should reject empty URL")
	}
}

func TestValidateURLAllowingFile_GatesFileScheme(t *testing.T) {
	const fileURL = "file:///tmp/pinchtab.html"

	if err := ValidateURLAllowingFile(fileURL, false); err == nil {
		t.Fatal("ValidateURLAllowingFile(file, false) should reject file:// scheme")
	}
	if err := ValidateURL(fileURL); err == nil {
		t.Fatal("ValidateURL should reject file:// scheme by default")
	}

	if err := ValidateURLAllowingFile(fileURL, true); err != nil {
		t.Fatalf("ValidateURLAllowingFile(file, true) error = %v", err)
	}

	for _, rawURL := range []string{"https://pinchtab.com", "http://pinchtab.test", "pinchtab.com", "about:blank"} {
		if err := ValidateURLAllowingFile(rawURL, true); err != nil {
			t.Fatalf("ValidateURLAllowingFile(%q, true) error = %v", rawURL, err)
		}
	}

	for _, rawURL := range []string{"javascript:alert(1)", "chrome://settings", "data:text/html,hello"} {
		if err := ValidateURLAllowingFile(rawURL, true); err == nil {
			t.Fatalf("ValidateURLAllowingFile(%q, true) should still reject", rawURL)
		}
	}
}

func TestIsFileURL(t *testing.T) {
	for _, rawURL := range []string{"file:///etc/passwd", "FILE:///tmp/x.html"} {
		if !IsFileURL(rawURL) {
			t.Fatalf("IsFileURL(%q) = false, want true", rawURL)
		}
	}
	for _, rawURL := range []string{"https://pinchtab.com", "http://pinchtab.test", "about:blank", "pinchtab.com"} {
		if IsFileURL(rawURL) {
			t.Fatalf("IsFileURL(%q) = true, want false", rawURL)
		}
	}
}

func TestValidateTarget_AllowsLocalHosts(t *testing.T) {
	v := &Validator{}
	for _, rawURL := range []string{
		"http://localhost:9867",
		"http://127.0.0.1:8080",
		"http://[::1]:9222",
		"http://foo.localhost:3000",
		"about:blank",
	} {
		target, err := v.ValidateTarget(context.Background(), rawURL, false)
		if err != nil {
			t.Fatalf("ValidateTarget(%q) error = %v", rawURL, err)
		}
		if target == nil || !target.AllowInternal {
			t.Fatalf("ValidateTarget(%q) should allow local targets", rawURL)
		}
	}
}

func TestParseCIDRs_DropsUnparseableKeepsValid(t *testing.T) {
	got := ParseCIDRs([]string{
		"10.0.0.0/8",  // valid CIDR
		"192.168.1.1", // bare IPv4 -> /32
		"::1",         // bare IPv6 -> /128
		"  ",          // blank, skipped silently
		"example.com", // hostname -> dropped (with warning)
		"junk/99",     // invalid CIDR -> dropped
	})
	if len(got) != 3 {
		t.Fatalf("ParseCIDRs returned %d nets, want 3 (valid entries only): %v", len(got), got)
	}
}

func TestValidateTarget_RejectsNoHostUnsafeSchemes(t *testing.T) {
	v := &Validator{}
	for _, rawURL := range []string{
		"file:///etc/passwd",
		"data:text/html,<script>alert(1)</script>",
		"javascript:alert(1)",
	} {
		if _, err := v.ValidateTarget(context.Background(), rawURL, false); err == nil {
			t.Fatalf("ValidateTarget(%q) should reject no-host unsafe scheme", rawURL)
		}
	}
}

func TestValidateTarget_AllowsNoHostSafeInputs(t *testing.T) {
	v := &Validator{}
	for _, rawURL := range []string{
		"about:blank",
		"some/relative/path",
		"",
	} {
		if _, err := v.ValidateTarget(context.Background(), rawURL, false); err != nil {
			t.Fatalf("ValidateTarget(%q) should allow safe hostless input, got %v", rawURL, err)
		}
	}
}

func TestValidateTarget_RejectsPrivateLiteralIP(t *testing.T) {
	v := &Validator{}
	if _, err := v.ValidateTarget(context.Background(), "http://192.168.1.10/app", false); err == nil {
		t.Fatal("ValidateTarget should reject private literal IPs")
	}
}

func TestValidateTarget_RejectsResolvedPrivateIP(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.168.1.10")}, nil
	})

	v := &Validator{}
	if _, err := v.ValidateTarget(context.Background(), "https://example.com/app", false); err == nil {
		t.Fatal("ValidateTarget should reject hosts resolving to private IPs")
	}
}

func TestValidateTarget_AllowsDualStackPublicResolution(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{
			net.ParseIP("2606:2800:220:1:248:1893:25c8:1946"),
			net.ParseIP("93.184.216.34"),
		}, nil
	})

	v := &Validator{}
	if _, err := v.ValidateTarget(context.Background(), "https://dual.example/app", false); err != nil {
		t.Fatalf("ValidateTarget should allow public dual-stack resolution: %v", err)
	}
}

func TestValidateTarget_RejectsMixedPublicPrivateDualStackResolution(t *testing.T) {
	tests := []struct {
		name string
		ips  []net.IP
	}{
		{
			name: "private-v6-first",
			ips:  []net.IP{net.ParseIP("fd00::10"), net.ParseIP("93.184.216.34")},
		},
		{
			name: "public-v4-first",
			ips:  []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("fd00::10")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
				return tt.ips, nil
			})

			v := &Validator{}
			if _, err := v.ValidateTarget(context.Background(), "https://mixed.example/app", false); err == nil {
				t.Fatal("ValidateTarget should reject mixed public/private DNS answers")
			}
		})
	}
}

func TestValidateTarget_AllowsResolvedPrivateIPWhenExplicitlyAllowlisted(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("172.18.0.5")}, nil
	})

	v := &Validator{}
	target, err := v.ValidateTarget(context.Background(), "http://fixtures:80/app", true)
	if err != nil {
		t.Fatalf("ValidateTarget should allow explicitly allowlisted private targets: %v", err)
	}
	if target == nil {
		t.Fatal("ValidateTarget returned nil target")
	}
	if target.AllowInternal {
		t.Fatal("allowlisted private targets should not disable redirect/internal-IP runtime guards")
	}
	if len(target.TrustedResolvedIP) != 1 || target.TrustedResolvedIP[0] != netip.MustParseAddr("172.18.0.5") {
		t.Fatalf("TrustedResolvedIP = %v, want [172.18.0.5]", target.TrustedResolvedIP)
	}
}

func TestValidateTarget_AllowsPrivateIPWithTrustedResolveCIDR(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	v := &Validator{TrustedResolveCIDRs: ParseCIDRs([]string{"10.0.0.0/8"})}
	target, err := v.ValidateTarget(context.Background(), "https://internal.example.com", false)
	if err != nil {
		t.Fatalf("expected trusted CIDR to allow private IP, got %v", err)
	}
	if target == nil || target.AllowInternal {
		t.Fatal("trusted CIDR override should not set AllowInternal (runtime guard should still be active)")
	}
	if len(target.TrustedResolvedIP) != 1 || target.TrustedResolvedIP[0] != netip.MustParseAddr("10.0.0.5") {
		t.Fatalf("expected exact trusted resolved IPs to be captured, got %v", target.TrustedResolvedIP)
	}
}

func TestValidateTarget_RejectsMixedUntrustedWithTrustedResolveCIDR(t *testing.T) {
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5"), net.ParseIP("192.168.1.1")}, nil
	})

	v := &Validator{TrustedResolveCIDRs: ParseCIDRs([]string{"10.0.0.0/8"})}
	if _, err := v.ValidateTarget(context.Background(), "https://mixed.example.com", false); err == nil {
		t.Fatal("expected mixed trusted/untrusted private IPs to be blocked")
	}
}

func TestValidateRemoteIP_AllowsExactTrustedResolvedIP(t *testing.T) {
	if err := ValidateRemoteIP("10.1.2.3", nil, []netip.Addr{netip.MustParseAddr("10.1.2.3")}); err != nil {
		t.Fatalf("expected exact trusted resolved IP to be allowed, got %v", err)
	}
}

func TestValidateRemoteIP_RejectsDifferentIPInSameCIDR(t *testing.T) {
	err := ValidateRemoteIP("10.1.2.4", nil, []netip.Addr{netip.MustParseAddr("10.1.2.3")})
	if err == nil {
		t.Fatal("expected different runtime IP in same CIDR to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked remote IP") {
		t.Fatalf("expected blocked remote IP error, got %v", err)
	}
}

func TestValidateRemoteIP_LoopbackProxyAllowed(t *testing.T) {
	trusted := BuildTrustedProxyCIDRs(true, nil)

	for _, ip := range []string{"127.0.0.1", "127.255.255.254", "::1"} {
		if err := ValidateRemoteIP(ip, trusted, nil); err != nil {
			t.Errorf("ValidateRemoteIP(%q) with TrustLoopbackProxy=true returned %v, want nil", ip, err)
		}
	}
}

func TestValidateRemoteIP_LoopbackProxyBlockedByDefault(t *testing.T) {
	trusted := BuildTrustedProxyCIDRs(false, nil)

	for _, ip := range []string{"127.0.0.1", "::1"} {
		if err := ValidateRemoteIP(ip, trusted, nil); err == nil {
			t.Errorf("ValidateRemoteIP(%q) with TrustLoopbackProxy=false returned nil, want SSRF block", ip)
		}
	}
}

func TestBuildTrustedProxyCIDRs_FlagOff(t *testing.T) {
	got := BuildTrustedProxyCIDRs(false, []string{"10.0.0.0/8"})
	if len(got) != 1 {
		t.Fatalf("got %d CIDRs, want 1 (only the configured one); got=%v", len(got), got)
	}
	if got[0].String() != "10.0.0.0/8" {
		t.Errorf("got[0] = %s, want 10.0.0.0/8", got[0].String())
	}
	if cidrsContainIP(got, net.ParseIP("127.0.0.1")) {
		t.Errorf("loopback 127.0.0.1 must not be trusted when TrustLoopbackProxy=false")
	}
}

func TestBuildTrustedProxyCIDRs_FlagOn(t *testing.T) {
	got := BuildTrustedProxyCIDRs(true, []string{"10.0.0.0/8"})
	if !cidrsContainIP(got, net.ParseIP("127.0.0.1")) {
		t.Errorf("127.0.0.1 must be trusted when TrustLoopbackProxy=true; got=%v", cidrStrings(got))
	}
	if !cidrsContainIP(got, net.ParseIP("::1")) {
		t.Errorf("::1 must be trusted when TrustLoopbackProxy=true; got=%v", cidrStrings(got))
	}
	if !cidrsContainIP(got, net.ParseIP("10.4.5.6")) {
		t.Errorf("configured CIDR 10.0.0.0/8 must remain trusted; got=%v", cidrStrings(got))
	}
	if cidrsContainIP(got, net.ParseIP("8.8.8.8")) {
		t.Errorf("public IP 8.8.8.8 must not be trusted; got=%v", cidrStrings(got))
	}
}

func TestBuildTrustedProxyCIDRs_NilCIDRs(t *testing.T) {
	got := BuildTrustedProxyCIDRs(false, nil)
	if len(got) != 0 {
		t.Fatalf("BuildTrustedProxyCIDRs(false, nil) = %v, want empty", got)
	}
}

func TestParseCIDRs_TreatsBareIPsAsSingleHosts(t *testing.T) {
	cidrs := ParseCIDRs([]string{"10.1.2.3", "fd00::1234"})
	if len(cidrs) != 2 {
		t.Fatalf("ParseCIDRs() returned %d entries, want 2", len(cidrs))
	}
	if got := cidrs[0].String(); got != "10.1.2.3/32" {
		t.Fatalf("IPv4 bare IP parsed as %q, want 10.1.2.3/32", got)
	}
	if got := cidrs[1].String(); got != "fd00::1234/128" {
		t.Fatalf("IPv6 bare IP parsed as %q, want fd00::1234/128", got)
	}
}

func TestValidateTarget_DomainAllowedCallbackPermitsPrivateIP(t *testing.T) {
	// Callers compute allowExplicitInternal from the IDPI domain allowlist
	// (handlers use idpi Guard.DomainAllowed); modeled here as a local callback.
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.168.1.100")}, nil
	})

	domainAllowed := func(rawURL string) bool { return true }
	v := &Validator{}
	rawURL := "https://allowed.corp.example.com/internal"
	allowInternal := domainAllowed(rawURL)
	target, err := v.ValidateTarget(context.Background(), rawURL, allowInternal)
	if err != nil {
		t.Fatalf("expected domain-allowed callback to permit private IP, got %v", err)
	}
	if target == nil {
		t.Fatal("expected non-nil target")
	}
	// allowExplicitInternal path sets TrustedResolvedIP, not AllowInternal
	if target.AllowInternal {
		t.Fatal("domain-allowed path should not set AllowInternal (that is reserved for localhost)")
	}
	if len(target.TrustedResolvedIP) != 1 || target.TrustedResolvedIP[0] != netip.MustParseAddr("192.168.1.100") {
		t.Fatalf("expected TrustedResolvedIP=[192.168.1.100], got %v", target.TrustedResolvedIP)
	}
}

func TestValidateTarget_DomainNotAllowedBlocksPrivateIP(t *testing.T) {
	// When the caller's domain allowlist says no AND no trusted CIDRs,
	// navigation to a private IP is blocked.
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.168.1.100")}, nil
	})

	domainAllowed := func(rawURL string) bool { return false }
	v := &Validator{}
	rawURL := "https://untrusted.example.com/internal"
	allowInternal := domainAllowed(rawURL)
	_, err := v.ValidateTarget(context.Background(), rawURL, allowInternal)
	if err == nil {
		t.Fatal("expected private IP to be blocked when the domain is not allowlisted and no trusted CIDRs")
	}
}

func TestValidateRemoteIP_PrivateIPBlockedWithNoCIDRs(t *testing.T) {
	// Remote IP is 10.0.0.1, no trusted CIDRs, no trusted resolved IPs.
	// Should return error.
	err := ValidateRemoteIP("10.0.0.1", nil, nil)
	if err == nil {
		t.Fatal("expected private remote IP to be blocked with no trusted CIDRs or resolved IPs")
	}
	if !strings.Contains(err.Error(), "blocked remote IP") {
		t.Fatalf("expected 'blocked remote IP' error message, got %v", err)
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
