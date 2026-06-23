package handlers

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/semantic/recovery"
)

// TestCacheActionIntent_DescriptorComposite asserts the recovery intent recorded
// for a ref yields the same Composite() (role/name/value) as before the switch
// from rebuilding the full descriptor slice to a direct single-node descriptor.
func TestCacheActionIntent_DescriptorComposite(t *testing.T) {
	mb := &visibleMockBridge{
		refCache: &bridge.RefCache{
			Nodes: []bridge.A11yNode{
				{Ref: "e1", Role: "link", Name: "Home"},
				{Ref: "e5", Role: "button", Name: "Submit"},
			},
		},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	h.Recovery = &recovery.RecoveryEngine{IntentCache: recovery.NewIntentCache(0, 0)}

	h.cacheActionIntent("tab1", bridge.ActionRequest{Ref: "e5"})

	entry, ok := h.Recovery.IntentCache.Lookup("tab1", "e5")
	if !ok {
		t.Fatal("expected intent entry to be recorded for e5")
	}
	if got := entry.Descriptor.Composite(); got != "button: Submit" {
		t.Fatalf("Composite() = %q, want %q", got, "button: Submit")
	}
}

// TestCacheActionIntent_UnknownRefKeepsRefDefault asserts the not-found default
// (a descriptor carrying just the ref) is preserved when the ref is absent.
func TestCacheActionIntent_UnknownRefKeepsRefDefault(t *testing.T) {
	mb := &visibleMockBridge{
		refCache: &bridge.RefCache{
			Nodes: []bridge.A11yNode{{Ref: "e1", Role: "link", Name: "Home"}},
		},
	}
	h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
	h.Recovery = &recovery.RecoveryEngine{IntentCache: recovery.NewIntentCache(0, 0)}

	h.cacheActionIntent("tab1", bridge.ActionRequest{Ref: "e99"})

	entry, ok := h.Recovery.IntentCache.Lookup("tab1", "e99")
	if !ok {
		t.Fatal("expected intent entry to be recorded for e99")
	}
	if entry.Descriptor.Ref != "e99" {
		t.Fatalf("Descriptor.Ref = %q, want %q", entry.Descriptor.Ref, "e99")
	}
	if got := entry.Descriptor.Composite(); got != "" {
		t.Fatalf("Composite() = %q, want empty for unknown ref", got)
	}
}
