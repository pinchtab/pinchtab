package engine

import (
	"context"
	"testing"
)

// fakeEngine implements Engine for testing.
type fakeEngine struct{ name string }

func (f *fakeEngine) Name() string                                                  { return f.name }
func (f *fakeEngine) Navigate(_ context.Context, _ string) (*NavigateResult, error) { return nil, nil }
func (f *fakeEngine) Snapshot(_ context.Context, _, _ string) (*SnapshotResult, error) {
	return nil, nil
}
func (f *fakeEngine) Text(_ context.Context, _ string) (*TextResult, error) { return nil, nil }
func (f *fakeEngine) Click(_ context.Context, _, _ string) error            { return nil }
func (f *fakeEngine) Type(_ context.Context, _, _, _ string) error          { return nil }
func (f *fakeEngine) Capabilities() []Capability                            { return nil }
func (f *fakeEngine) Close() error                                          { return nil }

func TestRouterChromeMode(t *testing.T) {
	r := NewRouter(ModeChrome, nil)
	for _, op := range []Capability{CapNavigate, CapSnapshot, CapText, CapScreenshot} {
		if r.UseLite(op, "https://example.com") {
			t.Errorf("chrome mode should never use lite for %s", op)
		}
	}
}

func TestRouterLiteMode(t *testing.T) {
	r := NewRouter(ModeLite, &fakeEngine{name: "lite"})

	// DOM operations → lite
	for _, op := range []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType} {
		if !r.UseLite(op, "https://example.com") {
			t.Errorf("lite mode should use lite for %s", op)
		}
	}
	// Chrome-only operations → chrome
	for _, op := range []Capability{CapScreenshot, CapPDF, CapEvaluate, CapCookies} {
		if r.UseLite(op, "https://example.com") {
			t.Errorf("lite mode should not use lite for %s", op)
		}
	}
}

func TestRouterAutoModeStaticContent(t *testing.T) {
	r := NewRouter(ModeAuto, &fakeEngine{name: "lite"})

	// Static HTML → lite (ContentHintRule)
	if !r.UseLite(CapNavigate, "https://example.com/page.html") {
		t.Error("auto mode should use lite for .html URL")
	}
	// Dynamic page → chrome (no matching rule)
	if r.UseLite(CapNavigate, "https://example.com/app") {
		t.Error("auto mode should use chrome for dynamic URL")
	}
	// Screenshot → always chrome
	if r.UseLite(CapScreenshot, "https://example.com/page.html") {
		t.Error("auto mode should use chrome for screenshot")
	}
}

func TestRouterAutoModeLiteNil(t *testing.T) {
	r := NewRouter(ModeAuto, nil)
	// Even if rule says lite, nil engine → falls through to chrome
	if r.UseLite(CapNavigate, "https://example.com/page.html") {
		t.Error("should not route to lite when lite engine is nil")
	}
}

func TestRouterAddRemoveRule(t *testing.T) {
	r := NewRouter(ModeAuto, &fakeEngine{name: "lite"})

	// Before: dynamic URL → chrome
	if r.UseLite(CapNavigate, "https://example.com/app") {
		t.Error("should route to chrome before custom rule")
	}

	// Add a custom rule that always routes navigate to lite
	r.AddRule(DefaultLiteRule{})

	// After: still chrome because CapabilityRule + ContentHintRule are first
	// and DefaultChromeRule is last. Our added rule goes before the fallback.
	if !r.UseLite(CapNavigate, "https://example.com/app") {
		t.Error("should route to lite after adding DefaultLiteRule")
	}

	// Remove the custom rule
	if !r.RemoveRule("default-lite") {
		t.Error("RemoveRule should return true")
	}

	if r.UseLite(CapNavigate, "https://example.com/app") {
		t.Error("should revert to chrome after removing rule")
	}
}

func TestRouterRulesSnapshot(t *testing.T) {
	r := NewRouter(ModeAuto, &fakeEngine{name: "lite"})
	rules := r.Rules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules in auto mode, got %d: %v", len(rules), rules)
	}
	if rules[0] != "capability" {
		t.Errorf("first rule should be capability, got %s", rules[0])
	}
	if rules[1] != "content-hint" {
		t.Errorf("second rule should be content-hint, got %s", rules[1])
	}
	if rules[2] != "default-chrome" {
		t.Errorf("third rule should be default-chrome, got %s", rules[2])
	}
}
