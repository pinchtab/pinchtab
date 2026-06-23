package runtime

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestInitBrowserFromExistingCDPRejectsNonAllowlistedHost(t *testing.T) {
	cfg := &config.RuntimeConfig{
		CDPAttachURL: "ws://203.0.113.10:9222/devtools/browser/abc",
	}

	_, _, _, _, _, err := initBrowserFromExistingCDP(cfg, nil)
	if err == nil {
		t.Fatal("expected non-loopback CDPAttachURL without allowlist to be rejected")
	}
	if !strings.Contains(err.Error(), "security.attach.allowHosts") {
		t.Fatalf("error should mention security.attach.allowHosts, got %v", err)
	}
}

func TestInitBrowserFromExistingCDPRejectsDisallowedScheme(t *testing.T) {
	cfg := &config.RuntimeConfig{
		CDPAttachURL:       "http://127.0.0.1:9222",
		AttachAllowSchemes: []string{"ws"},
	}

	_, _, _, _, _, err := initBrowserFromExistingCDP(cfg, nil)
	if err == nil {
		t.Fatal("expected disallowed attach scheme to be rejected")
	}
	if !strings.Contains(err.Error(), "scheme") || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error should mention disallowed scheme, got %v", err)
	}
}

// Loopback must keep working with empty allowlists: normalization passes and
// the failure (nothing is listening on port 1) comes from the attach dial,
// not the validator.
func TestInitBrowserFromExistingCDPAllowsLoopbackWithoutAllowlist(t *testing.T) {
	cfg := &config.RuntimeConfig{
		CDPAttachURL: "ws://127.0.0.1:1/devtools/browser/abc",
	}

	_, _, _, _, _, err := initBrowserFromExistingCDP(cfg, nil)
	if err == nil {
		t.Fatal("expected attach to fail: nothing is listening on 127.0.0.1:1")
	}
	if strings.Contains(err.Error(), "normalize cdpAttachUrl") {
		t.Fatalf("loopback URL should pass normalization, got %v", err)
	}
	if !strings.Contains(err.Error(), "failed to attach to CDP") {
		t.Fatalf("expected dial-stage attach failure, got %v", err)
	}
}
