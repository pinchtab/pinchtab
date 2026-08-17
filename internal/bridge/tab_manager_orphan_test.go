package bridge

import (
	"testing"

	"github.com/chromedp/cdproto/target"
)

func pageTarget(id string) *target.Info {
	return &target.Info{TargetID: target.ID(id), Type: TargetTypePage}
}

// The production failure: Chrome held 158 page targets on an instance
// configured with maxTabs: 8, because the cap only ever counted the
// in-process managed map. Untracked targets must be selectable for reaping.
func TestSelectOrphanTargets_PicksUntrackedOldestFirst(t *testing.T) {
	tracked := map[string]bool{"t-managed": true}
	// GetTargets returns newest-first.
	pages := []*target.Info{pageTarget("t-new"), pageTarget("t-managed"), pageTarget("t-old")}

	got := selectOrphanTargets(tracked, pages, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 orphans, got %d (%v)", len(got), got)
	}
	if got[0] != target.ID("t-old") {
		t.Fatalf("expected oldest orphan first, got %v", got)
	}
	for _, id := range got {
		if id == target.ID("t-managed") {
			t.Fatal("a tracked target must never be reaped as an orphan")
		}
	}
}

func TestSelectOrphanTargets_RespectsLimit(t *testing.T) {
	pages := []*target.Info{pageTarget("a"), pageTarget("b"), pageTarget("c")}
	got := selectOrphanTargets(map[string]bool{}, pages, 2)
	if len(got) != 2 {
		t.Fatalf("expected limit of 2, got %d", len(got))
	}
}

func TestSelectOrphanTargets_IgnoresNonPageTargets(t *testing.T) {
	pages := []*target.Info{
		{TargetID: target.ID("worker"), Type: "service_worker"},
		nil,
		pageTarget("page"),
	}
	got := selectOrphanTargets(map[string]bool{}, pages, 0)
	if len(got) != 1 || got[0] != target.ID("page") {
		t.Fatalf("expected only the page target, got %v", got)
	}
}

func TestTrackedCDPIDs(t *testing.T) {
	tm := &TabManager{tabs: map[string]*TabEntry{
		"tab_1": {CDPID: "raw1"},
		"tab_2": {CDPID: ""},
	}}
	got := tm.trackedCDPIDs()
	if !got["raw1"] || len(got) != 1 {
		t.Fatalf("unexpected tracked set: %v", got)
	}
}
