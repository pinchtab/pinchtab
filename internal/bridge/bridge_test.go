package bridge

import (
	"sync"
	"testing"
)

func newTestBridge() *Bridge {
	b := &Bridge{
		TabManager: &TabManager{
			tabs:      make(map[string]*TabEntry),
			snapshots: make(map[string]*RefCache),
		},
	}
	return b
}

func TestRefCacheConcurrency(t *testing.T) {
	b := newTestBridge()

	// Simulate concurrent reads/writes to snapshot cache
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tabID := "tab1"

			b.SetRefCache(tabID, &RefCache{Refs: map[string]int64{
				"e0": int64(i),
			}})

			cache := b.GetRefCache(tabID)
			if cache == nil {
				t.Error("cache should not be nil")
			}
		}(i)
	}
	wg.Wait()
}

func TestRefCacheLookup(t *testing.T) {
	b := newTestBridge()

	cache := b.GetRefCache("tab1")
	if cache != nil {
		t.Error("expected nil cache for unknown tab")
	}

	b.SetRefCache("tab1", &RefCache{Refs: map[string]int64{
		"e0": 100,
		"e1": 200,
	}})

	cache = b.GetRefCache("tab1")

	if nid, ok := cache.Refs["e0"]; !ok || nid != 100 {
		t.Errorf("e0 expected 100, got %d", nid)
	}
	if nid, ok := cache.Refs["e1"]; !ok || nid != 200 {
		t.Errorf("e1 expected 200, got %d", nid)
	}
	if _, ok := cache.Refs["e99"]; ok {
		t.Error("e99 should not exist")
	}
}
