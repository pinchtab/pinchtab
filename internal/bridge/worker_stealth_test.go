package bridge

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

// PR #580 made the launch path stop pinning --user-agent when no explicit
// custom UA is configured, so the page now uses Chrome's NATIVE userAgent
// (which can be any installed Chrome build, e.g. 146.x). The worker stealth
// parity path in applyWorkerStealth still calls
// BuildPersona(launchUA, cfg.BrowserVersion), which synthesizes a UA from
// the static configured major even when launchUA is empty — and then
// workerStealthParityScript overrides the worker's navigator.userAgent to
// that synthesized value. The result: page UA != worker UA. This test
// reproduces the regression.
func TestWorkerStealthParityDefersToNativeUAWhenNoCustomUA(t *testing.T) {
	cfg := &config.RuntimeConfig{BrowserVersion: "144.0.7559.133"}
	bundle := stealth.NewBundle(cfg, 1)

	launchUA := bundle.LaunchUserAgent()
	if launchUA != "" {
		t.Fatalf("precondition: with no custom UA, LaunchUserAgent must be empty, got %q", launchUA)
	}

	persona := workerStealthPersona(launchUA, cfg.BrowserVersion)
	if persona.UserAgent != "" {
		t.Fatalf("worker persona must defer to native nav.userAgent when no custom UA is configured; got %q (this is forced onto the worker and creates a page/worker UA mismatch when installed Chrome is not %s)", persona.UserAgent, cfg.BrowserVersion)
	}
	if persona.NavigatorPlatform != "" {
		t.Fatalf("worker persona must defer to native nav.platform when no custom UA is configured; got %q", persona.NavigatorPlatform)
	}

	script := workerStealthParityScript(persona)
	if strings.Contains(script, "Chrome/144.0.0.0") {
		t.Fatalf("worker stealth script embeds a static config-derived UA the page no longer pins:\n%s", script)
	}
}

// In HEADLESS mode the launch path pins --user-agent (otherwise Chrome's
// native userAgent leaks "HeadlessChrome"). Workers must carry that same
// pinned UA so the page and workers stay in lockstep.
func TestWorkerStealthParityAppliesPinnedUserAgentWhenHeadless(t *testing.T) {
	cfg := &config.RuntimeConfig{BrowserVersion: "144.0.7559.133", Headless: true}
	bundle := stealth.NewBundle(cfg, 1)

	launchUA := bundle.LaunchUserAgent()
	if launchUA == "" {
		t.Fatalf("precondition: headless launch must pin a non-empty UA, got %q", launchUA)
	}
	if strings.Contains(launchUA, "HeadlessChrome") {
		t.Fatalf("pinned headless UA must not contain HeadlessChrome, got %q", launchUA)
	}

	persona := workerStealthPersona(launchUA, cfg.BrowserVersion)
	if persona.UserAgent != launchUA {
		t.Fatalf("worker persona must carry the pinned headless UA, got %q want %q", persona.UserAgent, launchUA)
	}
}

// When an operator EXPLICITLY configures a custom UA, the worker must carry
// that same UA so it matches the page.
func TestWorkerStealthParityAppliesCustomUserAgent(t *testing.T) {
	customUA := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"
	cfg := &config.RuntimeConfig{BrowserVersion: "144.0.7559.133", UserAgent: customUA}
	bundle := stealth.NewBundle(cfg, 1)

	launchUA := bundle.LaunchUserAgent()
	if launchUA != customUA {
		t.Fatalf("precondition: explicit custom UA must be pinned, got %q", launchUA)
	}

	persona := workerStealthPersona(launchUA, cfg.BrowserVersion)
	if persona.UserAgent != customUA {
		t.Fatalf("worker persona must carry the explicit custom UA, got %q", persona.UserAgent)
	}
	script := workerStealthParityScript(persona)
	if !strings.Contains(script, customUA) {
		t.Fatalf("worker stealth script must embed the explicit custom UA:\n%s", script)
	}
}
