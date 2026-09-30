package bridge

import (
	"context"
	"testing"
)

func scopedTabManager(ids ...string) *TabManager {
	tm := NewTabManager(context.Background(), nil, nil, nil)
	for _, id := range ids {
		tm.tabs[id] = &TabEntry{Ctx: context.Background()}
	}
	return tm
}

func TestATabKeepsItsFirstCreatorAndStaysSharedOnceAnotherScopeUsesIt(t *testing.T) {
	tm := scopedTabManager("own", "shared", "returned", "recreated")
	tm.RecordTabScope("own", "session:s", true)
	tm.RecordTabScope("own", "session:s", false)
	tm.RecordTabScope("shared", "session:s", true)
	tm.RecordTabScope("shared", "session:t", false)
	tm.RecordTabScope("returned", "session:s", true)
	tm.RecordTabScope("returned", "session:t", false)
	tm.RecordTabScope("returned", "session:s", false)
	tm.RecordTabScope("recreated", "session:s", true)
	tm.RecordTabScope("recreated", "session:t", true)
	tm.RecordTabScope("missing", "session:s", true)

	got := tm.TabsOnlyUsedByCreator("session:s")
	if len(got) != 1 || got[0] != "own" {
		t.Fatalf("TabsOnlyUsedByCreator(session:s) = %v, want [own]: a tab another scope has used stays open even after its creator returns to it", got)
	}
	if got := tm.TabsOnlyUsedByCreator("session:t"); len(got) != 0 {
		t.Fatalf("TabsOnlyUsedByCreator(session:t) = %v, want none: t never created a tab", got)
	}
}
