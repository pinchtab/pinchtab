package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

// TestCacheActionIntent_DescriptorComposite asserts the recovery intent recorded
// for a ref yields the same Composite() (role/name/value) as before the switch
// from rebuilding the full descriptor slice to a direct single-node descriptor.
func TestCacheActionIntent_DescriptorComposite(t *testing.T) {
	mb := &mockBridge{
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
	mb := &mockBridge{
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

func findIntentFor(ref string) recovery.IntentEntry {
	return recovery.IntentEntry{
		Query:      "Add Blue Mug to cart",
		Descriptor: semantic.ElementDescriptor{Ref: ref, Role: "button", Name: "Add Blue Mug to cart"},
	}
}

func TestCacheActionIntentKeepsAFindIntentOnlyWhileTheRefStillNamesItsNode(t *testing.T) {
	cases := []struct {
		name      string
		node      bridge.A11yNode
		wantQuery string
	}{
		{"same node", bridge.A11yNode{Ref: "e1", Role: "button", Name: "Add Blue Mug to cart"}, "Add Blue Mug to cart"},
		{"different node", bridge.A11yNode{Ref: "e1", Role: "button", Name: "Load more"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mb := &mockBridge{refCache: &bridge.RefCache{Nodes: []bridge.A11yNode{tc.node}}}
			h := New(mb, &config.RuntimeConfig{}, nil, nil, nil)
			h.Recovery = &recovery.RecoveryEngine{IntentCache: recovery.NewIntentCache(0, 0)}
			h.Recovery.RecordIntent("tab1", "e1", findIntentFor("e1"))

			h.cacheActionIntent("tab1", bridge.ActionRequest{Ref: "e1"})

			entry, ok := h.Recovery.IntentCache.Lookup("tab1", "e1")
			if !ok {
				t.Fatal("expected an intent entry for e1")
			}
			if entry.Query != tc.wantQuery {
				t.Errorf("Query = %q, want %q", entry.Query, tc.wantQuery)
			}
			if got, want := entry.Descriptor.Name, tc.node.Name; got != want {
				t.Errorf("Descriptor.Name = %q, want %q", got, want)
			}
		})
	}
}

func TestARefActedOnAfterAFindOnAnotherPageRecoversAfterABareNavigation(t *testing.T) {
	_, b, h := newRerenderFixture(t)
	h.Recovery.RecordIntent("tab-rerender", "e0", findIntentFor("e0"))
	if w := postRerenderAction(t, h, `{"kind":"fill","ref":"e0","text":"first","tabId":"tab-rerender"}`); w.Code != http.StatusOK {
		t.Fatalf("fill on the new page: status=%d body=%s", w.Code, w.Body.String())
	}
	b.DeleteRefCache("tab-rerender")

	w := postRerenderAction(t, h, `{"kind":"fill","ref":"e0","text":"again","tabId":"tab-rerender"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("fill after a bare navigation: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Recovery recovery.RecoveryResult `json:"recovery"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	if !resp.Recovery.Recovered {
		t.Fatalf("recovery = %+v, want recovered", resp.Recovery)
	}
}
