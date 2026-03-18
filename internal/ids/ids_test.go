package ids

import "testing"

func TestTabIDFromCDPTarget_Passthrough(t *testing.T) {
	m := NewManager()
	cdpID := "A25658CE1BA82659EBE9C93C46CEE63A"
	got := m.TabIDFromCDPTarget(cdpID)
	if got != cdpID {
		t.Errorf("TabIDFromCDPTarget(%q) = %q, want %q", cdpID, got, cdpID)
	}
}
