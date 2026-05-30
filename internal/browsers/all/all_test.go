package all_test

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
)

func TestBuiltinProvidersRegistered(t *testing.T) {
	ids := browsers.IDs()

	if len(ids) < 3 {
		t.Fatalf("expected at least 3 registered providers, got %d: %v", len(ids), ids)
	}

	want := []string{"chrome", "cloak", "ghost-chrome"}
	for _, id := range want {
		b, ok := browsers.Get(id)
		if !ok {
			t.Errorf("browsers.Get(%q) = (_, false); want registered", id)
			continue
		}
		if b.ID() != id {
			t.Errorf("browser.ID() = %q; want %q", b.ID(), id)
		}
	}
}

func TestIDsSorted(t *testing.T) {
	ids := browsers.IDs()

	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			t.Fatalf("IDs() not sorted: %v", ids)
		}
	}
}

func TestMustGetBuiltins(t *testing.T) {
	for _, id := range []string{"chrome", "cloak", "ghost-chrome"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("MustGet(%q) panicked: %v", id, r)
				}
			}()
			b := browsers.MustGet(id)
			if b.ID() != id {
				t.Errorf("MustGet(%q).ID() = %q", id, b.ID())
			}
		}()
	}
}

func TestMustGetUnknownPanicsWithBuiltins(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustGet(\"nonexistent\") did not panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		for _, id := range []string{"chrome", "cloak", "ghost-chrome"} {
			if !strings.Contains(msg, id) {
				t.Errorf("panic message should list %q, got: %s", id, msg)
			}
		}
	}()
	browsers.MustGet("nonexistent")
}

func TestBuiltinCapabilityDifference(t *testing.T) {
	chrome := browsers.MustGet("chrome")
	cloak := browsers.MustGet("cloak")

	if chrome.Capabilities().Has(browsers.CapNativeStealth) {
		t.Error("Chrome should NOT have CapNativeStealth")
	}
	if !cloak.Capabilities().Has(browsers.CapNativeStealth) {
		t.Error("Cloak should have CapNativeStealth")
	}
}

func TestStableIDOrdering(t *testing.T) {
	ids := browsers.IDs()
	want := []string{"chrome", "cloak", "ghost-chrome"}

	if len(ids) != len(want) {
		t.Fatalf("IDs() = %v; want %v", ids, want)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("IDs()[%d] = %q; want %q (full: %v)", i, ids[i], id, ids)
		}
	}
}

func TestCloakCapabilitySupersetOfChrome(t *testing.T) {
	chromeCaps := browsers.MustGet("chrome").Capabilities().List()
	cloakCaps := browsers.MustGet("cloak").Capabilities()

	// Cloak intentionally lacks CapEventScreencast because its CDP proxy
	// does not support Page.startScreencast — polling is used instead.
	chromeOnly := map[browsers.BrowserCapability]bool{
		browsers.CapEventScreencast: true,
	}

	for _, c := range chromeCaps {
		if chromeOnly[c] {
			continue
		}
		if !cloakCaps.Has(c) {
			t.Errorf("Cloak missing Chrome capability %q", c)
		}
	}

	if !cloakCaps.Has(browsers.CapNativeStealth) {
		t.Error("Cloak should have CapNativeStealth beyond Chrome's set")
	}
	if cloakCaps.Has(browsers.CapEventScreencast) {
		t.Error("Cloak should NOT have CapEventScreencast")
	}
}

func TestCDPSupportConsistency(t *testing.T) {
	for _, id := range []string{"chrome", "cloak", "ghost-chrome"} {
		b := browsers.MustGet(id)
		if !b.SupportsRemoteCDP() {
			t.Errorf("%s: SupportsRemoteCDP() = false; want true", id)
		}
	}
}
