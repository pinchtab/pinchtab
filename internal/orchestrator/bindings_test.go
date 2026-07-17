package orchestrator

import (
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestBindings_SessionRoundTrip(t *testing.T) {
	b := NewBindings(nil)
	b.BindSession("ses_1", "inst_a")
	if got, ok := b.ResolveSession("ses_1"); !ok || got != "inst_a" {
		t.Fatalf("ResolveSession = %q, %v; want inst_a, true", got, ok)
	}
	b.ClearSession("ses_1")
	if _, ok := b.ResolveSession("ses_1"); ok {
		t.Fatal("expected session binding cleared")
	}
}

func TestBindings_SessionTabsAreIsolatedAndReleased(t *testing.T) {
	b := NewBindings(nil)
	b.OwnSessionTab("ses_1", "inst_a", "tab_b")
	b.OwnSessionTab("ses_1", "inst_a", "tab_a")
	b.OwnSessionTab("ses_2", "inst_b", "tab_c")

	if got := b.SessionTabIDs("ses_1"); len(got) != 2 || got[0] != "tab_a" || got[1] != "tab_b" {
		t.Fatalf("ses_1 tabs = %v, want [tab_a tab_b]", got)
	}
	if got := b.SessionTabIDs("ses_2"); len(got) != 1 || got[0] != "tab_c" {
		t.Fatalf("ses_2 tabs = %v, want [tab_c]", got)
	}

	b.ReleaseTab("tab_a")
	if got := b.SessionTabIDs("ses_1"); len(got) != 1 || got[0] != "tab_b" {
		t.Fatalf("ses_1 tabs after close = %v, want [tab_b]", got)
	}
	if got := b.SessionTabIDs("ses_2"); len(got) != 1 || got[0] != "tab_c" {
		t.Fatalf("closing ses_1 tab changed ses_2 tabs: %v", got)
	}

	b.ClearSession("ses_1")
	if got := b.SessionTabIDs("ses_1"); len(got) != 0 {
		t.Fatalf("cleared session tabs = %v, want empty", got)
	}
	if got := b.SessionTabIDs("ses_2"); len(got) != 1 || got[0] != "tab_c" {
		t.Fatalf("clearing ses_1 changed ses_2 tabs: %v", got)
	}
}

func TestBindings_AgentResolveBumpsIdle(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	b := NewBindings(func() time.Time { return clock })
	b.BindAgent("agent-1", "inst_a")

	clock = t0.Add(30 * time.Minute)
	if _, ok := b.ResolveAgent("agent-1"); !ok {
		t.Fatal("expected agent binding to resolve")
	}
	clock = t0.Add(90 * time.Minute) // would be older than 1h cutoff from t0
	b.PruneAgents(1*time.Hour, 0)
	if _, ok := b.ResolveAgent("agent-1"); !ok {
		t.Fatal("Resolve should have bumped idle past cutoff and survived prune")
	}
}

func TestBindings_PruneIdleAgents(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	b := NewBindings(func() time.Time { return clock })

	b.BindAgent("stale", "inst_a")
	clock = t0.Add(2 * time.Hour)
	b.BindAgent("fresh", "inst_b")

	b.PruneAgents(1*time.Hour, 0)

	if _, ok := b.ResolveAgent("stale"); ok {
		t.Fatal("stale agent should have been pruned")
	}
	if _, ok := b.ResolveAgent("fresh"); !ok {
		t.Fatal("fresh agent should have survived")
	}
}

func TestBindings_PruneLRUCap(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	b := NewBindings(func() time.Time { return clock })

	for i, id := range []string{"a", "b", "c", "d"} {
		clock = t0.Add(time.Duration(i) * time.Minute)
		b.BindAgent(id, "inst_x")
	}
	b.PruneAgents(0, 2)

	if _, ok := b.ResolveAgent("a"); ok {
		t.Fatal("oldest 'a' should be evicted by LRU cap")
	}
	if _, ok := b.ResolveAgent("b"); ok {
		t.Fatal("oldest 'b' should be evicted by LRU cap")
	}
	if _, ok := b.ResolveAgent("c"); !ok {
		t.Fatal("'c' should survive cap")
	}
	if _, ok := b.ResolveAgent("d"); !ok {
		t.Fatal("'d' should survive cap")
	}
}

func TestBindings_ClearInstanceDropsEveryReference(t *testing.T) {
	b := NewBindings(nil)
	b.BindSession("ses_1", "inst_a")
	b.BindSession("ses_2", "inst_b")
	b.BindAgent("agent-1", "inst_a")
	b.BindAgent("agent-2", "inst_b")
	b.OwnSessionTab("ses_1", "inst_a", "tab-a")
	b.OwnSessionTab("ses_2", "inst_b", "tab-b")

	b.ClearInstance("inst_a")

	if _, ok := b.ResolveSession("ses_1"); ok {
		t.Fatal("session pointing at cleared instance should be dropped")
	}
	if _, ok := b.ResolveAgent("agent-1"); ok {
		t.Fatal("agent pointing at cleared instance should be dropped")
	}
	if _, ok := b.ResolveSession("ses_2"); !ok {
		t.Fatal("session for surviving instance should remain")
	}
	if _, ok := b.ResolveAgent("agent-2"); !ok {
		t.Fatal("agent for surviving instance should remain")
	}
	if got := b.SessionTabIDs("ses_1"); len(got) != 0 {
		t.Fatalf("tabs for cleared instance = %v, want empty", got)
	}
	if got := b.SessionTabIDs("ses_2"); len(got) != 1 || got[0] != "tab-b" {
		t.Fatalf("tabs for surviving instance = %v, want [tab-b]", got)
	}
}

func TestBindings_NilSafe(t *testing.T) {
	var b *Bindings
	b.BindSession("a", "b")
	b.BindAgent("a", "b")
	if _, ok := b.ResolveSession("a"); ok {
		t.Fatal("nil should resolve nothing")
	}
	if _, ok := b.ResolveAgent("a"); ok {
		t.Fatal("nil should resolve nothing")
	}
	b.ClearInstance("anything")
	b.OwnSessionTab("a", "b", "c")
	b.ReleaseTab("c")
	if got := b.SessionTabIDs("a"); len(got) != 0 {
		t.Fatalf("nil SessionTabIDs = %v, want empty", got)
	}
	b.PruneAgents(time.Hour, 1)
}

func TestOrchestrator_InstanceStoppedClearsBindings(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{})
	o.bindings.BindSession("ses_1", "inst_a")
	o.bindings.BindAgent("agent-1", "inst_a")

	o.EmitEvent("instance.stopped", &bridge.Instance{ID: "inst_a"})

	if _, ok := o.bindings.ResolveSession("ses_1"); ok {
		t.Fatal("instance.stopped should clear session bindings")
	}
	if _, ok := o.bindings.ResolveAgent("agent-1"); ok {
		t.Fatal("instance.stopped should clear agent bindings")
	}
}
