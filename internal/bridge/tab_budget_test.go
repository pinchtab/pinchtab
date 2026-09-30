package bridge

import (
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"testing"

	"github.com/chromedp/cdproto/target"
)

func pageTarget(id string) *target.Info {
	return &target.Info{TargetID: target.ID(id), Type: bridgecdpops.TargetTypePage}
}

func TestSelectOrphanTargets_PicksUntrackedOldestFirst(t *testing.T) {
	tracked := map[string]bool{"t-managed": true}
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

func TestSelectOrphanTargets_LimitSparesNewest(t *testing.T) {
	pages := []*target.Info{pageTarget("in-flight"), pageTarget("mid"), pageTarget("oldest")}
	got := selectOrphanTargets(map[string]bool{}, pages, 1)
	if len(got) != 1 || got[0] != target.ID("oldest") {
		t.Fatalf("expected only the oldest target, got %v", got)
	}
	for _, id := range got {
		if id == target.ID("in-flight") {
			t.Fatal("a bounded reconcile must not reap the newest (in-flight) target")
		}
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
