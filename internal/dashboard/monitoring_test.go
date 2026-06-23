package dashboard

import (
	"sync"
	"testing"
	"time"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

type fakeMonSource struct {
	mu        sync.Mutex
	listCalls int
	metrics   []apiTypes.InstanceMetrics
}

func (f *fakeMonSource) List() []bridge.Instance {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return []bridge.Instance{}
}

func (f *fakeMonSource) AllTabs() []bridge.InstanceTab { return []bridge.InstanceTab{} }

func (f *fakeMonSource) AllMetrics() []apiTypes.InstanceMetrics { return f.metrics }

func (f *fakeMonSource) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func TestMonitoringPayloadCachesWithinTTL(t *testing.T) {
	d := NewDashboard(nil)
	base := time.Unix(1000, 0)
	cur := base
	d.now = func() time.Time { return cur }

	src := &fakeMonSource{}
	d.SetMonitoringSource(src)

	b1 := d.monitoringPayloadBytes(false)
	b2 := d.monitoringPayloadBytes(false)
	if got := src.calls(); got != 1 {
		t.Fatalf("List called %d times within TTL, want 1", got)
	}
	if string(b1) != string(b2) {
		t.Fatalf("cached bytes differ within TTL:\n%s\n%s", b1, b2)
	}

	cur = base.Add(monitoringCacheTTL + time.Millisecond)
	_ = d.monitoringPayloadBytes(false)
	if got := src.calls(); got != 2 {
		t.Fatalf("List called %d times after TTL, want 2 (recompute)", got)
	}
}

func TestMonitoringPayloadKeyedByIncludeMemory(t *testing.T) {
	d := NewDashboard(nil)
	cur := time.Unix(2000, 0)
	d.now = func() time.Time { return cur }

	// One metric so the includeMemory=true payload carries Metrics and the
	// includeMemory=false one does not — distinct bytes, cached independently.
	src := &fakeMonSource{metrics: []apiTypes.InstanceMetrics{{}}}
	d.SetMonitoringSource(src)

	withMem := d.monitoringPayloadBytes(true)
	withoutMem := d.monitoringPayloadBytes(false)
	if string(withMem) == string(withoutMem) {
		t.Fatalf("includeMemory variants should differ:\n%s", withMem)
	}
	if got := src.calls(); got != 2 {
		t.Fatalf("List called %d times, want 2 (one per includeMemory key)", got)
	}

	// Re-fetch within TTL serves both cached keys — no further recompute.
	_ = d.monitoringPayloadBytes(true)
	_ = d.monitoringPayloadBytes(false)
	if got := src.calls(); got != 2 {
		t.Fatalf("re-fetch within TTL recomputed: List called %d, want 2", got)
	}
}
