package main

import (
	"sync"
	"testing"
)

func TestRefCacheConcurrency(t *testing.T) {
	b := newTestBridgeWithTabs()

	// Simulate concurrent reads/writes to snapshot cache
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tabID := "tab1"

			b.mu.Lock()
			b.snapshots[tabID] = &refCache{refs: map[string]int64{
				"e0": int64(i),
			}}
			b.mu.Unlock()

			b.mu.RLock()
			cache := b.snapshots[tabID]
			b.mu.RUnlock()
			if cache == nil {
				t.Error("cache should not be nil")
			}
		}(i)
	}
	wg.Wait()
}

func TestRefCacheLookup(t *testing.T) {
	b := newTestBridgeWithTabs()

	b.mu.RLock()
	cache := b.snapshots["tab1"]
	b.mu.RUnlock()
	if cache != nil {
		t.Error("expected nil cache for unknown tab")
	}

	b.mu.Lock()
	b.snapshots["tab1"] = &refCache{refs: map[string]int64{
		"e0": 100,
		"e1": 200,
	}}
	b.mu.Unlock()

	b.mu.RLock()
	cache = b.snapshots["tab1"]
	b.mu.RUnlock()

	if nid, ok := cache.refs["e0"]; !ok || nid != 100 {
		t.Errorf("e0 expected 100, got %d", nid)
	}
	if nid, ok := cache.refs["e1"]; !ok || nid != 200 {
		t.Errorf("e1 expected 200, got %d", nid)
	}
	if _, ok := cache.refs["e99"]; ok {
		t.Error("e99 should not exist")
	}
}
