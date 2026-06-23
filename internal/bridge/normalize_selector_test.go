package bridge

import (
	"testing"
)

func TestNormalizeSelector_RefPromotion(t *testing.T) {
	// When Ref is set and Selector is empty, Ref should promote to Selector.
	req := ActionRequest{Ref: "e5"}
	req.NormalizeSelector()
	if req.Selector != "e5" {
		t.Errorf("after NormalizeSelector: Selector = %q, want %q", req.Selector, "e5")
	}
}

func TestNormalizeSelector_RefPromotionVariousRefs(t *testing.T) {
	refs := []string{"e0", "e1", "e42", "e99999"}
	for _, ref := range refs {
		req := ActionRequest{Ref: ref}
		req.NormalizeSelector()
		if req.Selector != ref {
			t.Errorf("NormalizeSelector(Ref=%q): Selector = %q, want %q", ref, req.Selector, ref)
		}
	}
}

func TestNormalizeSelector_SelectorPreserved(t *testing.T) {
	// When Selector is already set, it should not be overwritten.
	req := ActionRequest{Selector: "#login"}
	req.NormalizeSelector()
	if req.Selector != "#login" {
		t.Errorf("after NormalizeSelector: Selector = %q, want %q", req.Selector, "#login")
	}
}

func TestNormalizeSelector_SelectorPreservedVariousTypes(t *testing.T) {
	selectors := []string{
		"#login",
		".btn.primary",
		"div > span",
		"//div[@id='main']",
		"button",
		"e5", // even if it looks like a ref, if it's in Selector field, keep it
	}
	for _, sel := range selectors {
		req := ActionRequest{Selector: sel}
		req.NormalizeSelector()
		if req.Selector != sel {
			t.Errorf("NormalizeSelector(Selector=%q): Selector = %q, want %q", sel, req.Selector, sel)
		}
	}
}

func TestNormalizeSelector_RefWinsWhenBothSet(t *testing.T) {
	// When both Ref and Selector are set, Selector is already populated
	// so NormalizeSelector leaves it as-is (Ref doesn't overwrite).
	// This tests the documented "Ref > Selector" priority — but the
	// implementation only promotes Ref when Selector is empty.
	req := ActionRequest{Ref: "e5", Selector: "#login"}
	req.NormalizeSelector()
	// Selector stays as "#login" because it was already set
	if req.Selector != "#login" {
		t.Errorf("after NormalizeSelector: Selector = %q, want %q", req.Selector, "#login")
	}
}

func TestNormalizeSelector_BothEmpty(t *testing.T) {
	req := ActionRequest{}
	req.NormalizeSelector()
	if req.Selector != "" {
		t.Errorf("after NormalizeSelector: Selector = %q, want empty", req.Selector)
	}
	if req.Ref != "" {
		t.Errorf("after NormalizeSelector: Ref = %q, want empty", req.Ref)
	}
}

func TestNormalizeSelector_EmptyRef(t *testing.T) {
	req := ActionRequest{Ref: "", Selector: ".btn"}
	req.NormalizeSelector()
	if req.Selector != ".btn" {
		t.Errorf("after NormalizeSelector: Selector = %q, want %q", req.Selector, ".btn")
	}
}

func TestNormalizeSelector_Idempotent(t *testing.T) {
	// Calling NormalizeSelector multiple times should produce the same result.
	tests := []ActionRequest{
		{Ref: "e5"},
		{Selector: "#login"},
		{Ref: "e42", Selector: ""},
		{},
		{Ref: "e10", Selector: ".btn"},
	}
	for _, req := range tests {
		r1 := req
		r1.NormalizeSelector()
		sel1 := r1.Selector
		ref1 := r1.Ref

		r1.NormalizeSelector()
		if r1.Selector != sel1 {
			t.Errorf("idempotency failed: Selector changed from %q to %q", sel1, r1.Selector)
		}
		if r1.Ref != ref1 {
			t.Errorf("idempotency failed: Ref changed from %q to %q", ref1, r1.Ref)
		}

		// Third call for good measure
		r1.NormalizeSelector()
		if r1.Selector != sel1 {
			t.Errorf("idempotency failed on 3rd call: Selector = %q, want %q", r1.Selector, sel1)
		}
	}
}

func TestNormalizeSelector_OtherFieldsUntouched(t *testing.T) {
	req := ActionRequest{
		Ref:    "e5",
		Kind:   "click",
		Text:   "hello",
		Key:    "Enter",
		TabID:  "tab1",
		NodeID: 42,
		X:      1.5,
		Y:      2.5,
		HasXY:  true,
	}
	req.NormalizeSelector()

	if req.Kind != "click" {
		t.Errorf("Kind changed: %q", req.Kind)
	}
	if req.Text != "hello" {
		t.Errorf("Text changed: %q", req.Text)
	}
	if req.Key != "Enter" {
		t.Errorf("Key changed: %q", req.Key)
	}
	if req.TabID != "tab1" {
		t.Errorf("TabID changed: %q", req.TabID)
	}
	if req.NodeID != 42 {
		t.Errorf("NodeID changed: %d", req.NodeID)
	}
	if req.X != 1.5 || req.Y != 2.5 {
		t.Errorf("X/Y changed: %f, %f", req.X, req.Y)
	}
	if !req.HasXY {
		t.Error("HasXY changed")
	}
	// Ref should still be "e5" (not cleared)
	if req.Ref != "e5" {
		t.Errorf("Ref changed: %q", req.Ref)
	}
}

func TestNormalizeSelector_NonRefValueInRefField(t *testing.T) {
	// Some legacy callers might put a CSS selector in the Ref field.
	// NormalizeSelector should still promote it to Selector.
	req := ActionRequest{Ref: "#login"}
	req.NormalizeSelector()
	if req.Selector != "#login" {
		t.Errorf("after NormalizeSelector: Selector = %q, want %q", req.Selector, "#login")
	}
}
