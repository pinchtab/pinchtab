package orchestrator

import (
	"testing"
	"time"
)

// The maintenance tick is the only thing that bounds the tab→instance cache:
// tabs closed outside the proxy (bridge idle eviction, window.close) are never
// invalidated, so without this the cache only grows.
func TestRunMaintenanceOnceReconcilesTabCache(t *testing.T) {
	o := NewOrchestrator(t.TempDir())

	o.instanceMgr.RememberTab("tab-gone", "inst-gone")
	if got := o.instanceMgr.CachedTabCount(); got != 1 {
		t.Fatalf("cache size = %d, want 1", got)
	}

	o.runMaintenanceOnce(time.Hour, 10000)

	if got := o.instanceMgr.CachedTabCount(); got != 0 {
		t.Errorf("cache size after maintenance = %d, want 0 — no instances are running, so nothing should remain cached", got)
	}
}

func withInstanceManager(o *Orchestrator) *Orchestrator {
	o.initInstanceManager()
	return o
}
