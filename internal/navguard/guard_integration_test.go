package navguard

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

// Integration tests that exercise the full Validator pipeline:
// ValidateURL → ValidateTarget (DNS resolution) → ValidateRemoteIP.
// These scenarios document the 5 key security invariants that must hold
// when engine-level security code is removed.

func TestIntegration_FullPipeline_PublicURLAllowed(t *testing.T) {
	// Public URL → ValidateURL passes → ValidateTarget resolves to public IP → allowed
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})

	v := &Validator{}

	rawURL := "https://example.com/page"
	if err := v.ValidateURL(rawURL); err != nil {
		t.Fatalf("ValidateURL failed: %v", err)
	}

	target, err := v.ValidateTarget(context.Background(), rawURL, false)
	if err != nil {
		t.Fatalf("ValidateTarget failed: %v", err)
	}
	if target == nil {
		t.Fatal("expected non-nil target")
	}
	if target.AllowInternal {
		t.Fatal("public URL should not set AllowInternal")
	}
	if len(target.TrustedResolvedIP) != 0 {
		t.Fatalf("public URL should have no TrustedResolvedIP, got %v", target.TrustedResolvedIP)
	}
}

func TestIntegration_FullPipeline_PrivateIPBlocked(t *testing.T) {
	// Public URL → ValidateURL passes → ValidateTarget resolves to 192.168.1.1 → blocked
	// IDPIDomainAllowed returns false (simulated by passing allowExplicitInternal=false)
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.168.1.1")}, nil
	})

	v := &Validator{
		IDPIDomainAllowed: func(rawURL string) bool { return false },
	}

	rawURL := "https://internal-service.example.com/api"
	if err := v.ValidateURL(rawURL); err != nil {
		t.Fatalf("ValidateURL failed: %v", err)
	}

	// Simulate the caller checking IDPIDomainAllowed to decide allowExplicitInternal
	allowInternal := v.IDPIDomainAllowed(rawURL)
	_, err := v.ValidateTarget(context.Background(), rawURL, allowInternal)
	if err == nil {
		t.Fatal("expected ValidateTarget to block private IP when IDPIDomainAllowed=false")
	}
}

func TestIntegration_FullPipeline_AllowedDomainPermitsPrivateIP(t *testing.T) {
	// URL resolves to private IP BUT IDPIDomainAllowed returns true → allowed
	// This is the "allowed domain permits navigation" scenario
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.168.1.1")}, nil
	})

	v := &Validator{
		IDPIDomainAllowed: func(rawURL string) bool { return true },
	}

	rawURL := "https://trusted-internal.corp.example.com/dashboard"
	if err := v.ValidateURL(rawURL); err != nil {
		t.Fatalf("ValidateURL failed: %v", err)
	}

	// Caller uses IDPIDomainAllowed to decide allowExplicitInternal
	allowInternal := v.IDPIDomainAllowed(rawURL)
	target, err := v.ValidateTarget(context.Background(), rawURL, allowInternal)
	if err != nil {
		t.Fatalf("expected allowed domain to permit private IP, got %v", err)
	}
	if target == nil {
		t.Fatal("expected non-nil target")
	}
	// When allowExplicitInternal=true, TrustedResolvedIP is set but AllowInternal is NOT set
	// (AllowInternal is only set for localhost-class URLs)
	if target.AllowInternal {
		t.Fatal("allowed domain via IDPIDomainAllowed should not set AllowInternal (only localhost does)")
	}
	if len(target.TrustedResolvedIP) != 1 || target.TrustedResolvedIP[0] != netip.MustParseAddr("192.168.1.1") {
		t.Fatalf("expected TrustedResolvedIP=[192.168.1.1], got %v", target.TrustedResolvedIP)
	}
}

func TestIntegration_FullPipeline_RemoteIPValidation(t *testing.T) {
	// After navigation starts, remote IP comes back as 10.0.0.1
	// No trusted CIDRs → ValidateRemoteIP blocks
	v := &Validator{
		TrustedProxyCIDRs: nil,
	}

	err := v.ValidateRemoteIP("10.0.0.1", nil)
	if err == nil {
		t.Fatal("expected ValidateRemoteIP to block private remote IP with no trusted CIDRs")
	}
}

func TestIntegration_FullPipeline_TrustedCIDRBypass(t *testing.T) {
	// URL resolves to 10.0.0.5
	// TrustedResolveCIDRs includes 10.0.0.0/24
	// IDPIDomainAllowed returns false (no domain allowlist)
	// Should be allowed via CIDR bypass
	stubHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	})

	v := &Validator{
		TrustedResolveCIDRs: ParseCIDRs([]string{"10.0.0.0/24"}),
		IDPIDomainAllowed:   func(rawURL string) bool { return false },
	}

	rawURL := "https://k8s-service.internal.example.com/health"
	if err := v.ValidateURL(rawURL); err != nil {
		t.Fatalf("ValidateURL failed: %v", err)
	}

	// Even though IDPIDomainAllowed returns false, TrustedResolveCIDRs should allow
	allowInternal := v.IDPIDomainAllowed(rawURL) // false
	target, err := v.ValidateTarget(context.Background(), rawURL, allowInternal)
	if err != nil {
		t.Fatalf("expected TrustedResolveCIDRs to bypass private IP block, got %v", err)
	}
	if target == nil {
		t.Fatal("expected non-nil target")
	}
	if target.AllowInternal {
		t.Fatal("CIDR bypass should not set AllowInternal")
	}
	if len(target.TrustedResolvedIP) != 1 || target.TrustedResolvedIP[0] != netip.MustParseAddr("10.0.0.5") {
		t.Fatalf("expected TrustedResolvedIP=[10.0.0.5], got %v", target.TrustedResolvedIP)
	}

	// Verify that the remote IP validation also passes for this trusted resolved IP
	if err := v.ValidateRemoteIP("10.0.0.5", target.TrustedResolvedIP); err != nil {
		t.Fatalf("expected runtime remote IP check to pass for trusted resolved IP, got %v", err)
	}
}
