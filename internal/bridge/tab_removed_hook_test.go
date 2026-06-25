package bridge

import (
	"context"
	"testing"
)

// TestExternalTabRemovedHookSurvivesRewire verifies that a hook registered via
// Bridge.AddTabRemovedHook is applied to the current TabManager, re-applied when
// wireTabManager swaps the TabManager (launch/reinit/remote-CDP), and not
// duplicated across rewires — alongside the built-in dropFetchPauseSuppression.
func TestExternalTabRemovedHookSurvivesRewire(t *testing.T) {
	b := &Bridge{}

	var calls int
	b.AddTabRemovedHook(func(string) { calls++ })

	ctx := context.Background()
	b.wireTabManager(ctx)

	// External hook + built-in dropFetchPauseSuppression.
	if got := len(b.onTabRemovedHooks); got != 2 {
		t.Fatalf("hooks after first wire = %d, want 2", got)
	}
	for _, h := range b.onTabRemovedHooks {
		h("tab1")
	}
	if calls != 1 {
		t.Fatalf("external hook fired %d times, want 1", calls)
	}

	// A reinit swaps the TabManager; the external hook must persist without
	// duplicating (built-in is freshly re-added, not accumulated).
	b.wireTabManager(ctx)
	if got := len(b.onTabRemovedHooks); got != 2 {
		t.Fatalf("hooks after rewire = %d, want 2 (no duplication)", got)
	}
}
