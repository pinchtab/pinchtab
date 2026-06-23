package browsers

import "testing"

func TestNewCapabilitySetEmpty(t *testing.T) {
	cs := NewCapabilitySet()
	if cs.Len() != 0 {
		t.Fatalf("expected Len()==0, got %d", cs.Len())
	}
}

func TestNewCapabilitySetWithCaps(t *testing.T) {
	cs := NewCapabilitySet(CapCDP, CapHeadless)
	if cs.Len() != 2 {
		t.Fatalf("expected Len()==2, got %d", cs.Len())
	}
	if !cs.Has(CapCDP) {
		t.Fatal("expected set to contain CapCDP")
	}
	if !cs.Has(CapHeadless) {
		t.Fatal("expected set to contain CapHeadless")
	}
}

func TestHasReturnsFalseForAbsent(t *testing.T) {
	cs := NewCapabilitySet(CapCDP)
	if cs.Has(CapPDF) {
		t.Fatal("expected Has(CapPDF)==false for a set that does not contain it")
	}
}

func TestHasSafeOnEmptySet(t *testing.T) {
	cs := NewCapabilitySet()
	if cs.Has(CapCDP) {
		t.Fatal("expected Has()==false on empty set")
	}
}

func TestDuplicatesAreDeduplicated(t *testing.T) {
	cs := NewCapabilitySet(CapCDP, CapCDP)
	if cs.Len() != 1 {
		t.Fatalf("expected Len()==1 after dedup, got %d", cs.Len())
	}
}

func TestListReturnsSorted(t *testing.T) {
	// Pick capabilities whose string values are not in insertion order.
	cs := NewCapabilitySet(CapPDF, CapCDP, CapHeadless)
	list := cs.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}
	// Expected alphabetical order: cdp, headless, pdf
	want := []BrowserCapability{CapCDP, CapHeadless, CapPDF}
	for i, c := range want {
		if list[i] != c {
			t.Fatalf("index %d: expected %q, got %q", i, c, list[i])
		}
	}
}

func TestListEmptySet(t *testing.T) {
	cs := NewCapabilitySet()
	list := cs.List()
	if list == nil {
		t.Fatal("List() should return empty slice, not nil")
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 items, got %d", len(list))
	}
}
