package bridge

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
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

func TestTabManagerRemoteAllocatorInitialization(t *testing.T) {
	// Test that TabManager can be initialized without a valid browser context.
	// This is the case for remote allocators (CDP_URL mode) where the browser
	// context is established lazily.
	cfg := &config.RuntimeConfig{
		CdpURL: "ws://localhost:9222/devtools/browser/test",
	}

	// Use context.TODO() instead of nil to avoid lint warnings
	ctx := context.TODO()
	tm := NewTabManager(ctx, cfg, nil, nil)
	if tm == nil {
		t.Error("TabManager should be created")
	}

	// Attempting to create a tab with an invalid context should fail gracefully
	_, _, _, err := tm.CreateTab("about:blank")
	if err == nil {
		t.Error("CreateTab should fail when browserCtx is invalid")
	}
}

func TestTabContext_RejectsUnknownTabID(t *testing.T) {
	// TabContext should reject tab IDs that aren't tracked
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil)

	// Try to get context for a non-existent tab
	_, _, err := tm.TabContext("tab_nonexistent")
	if err == nil {
		t.Error("TabContext should reject unknown tab IDs")
	}
	if err.Error() != "tab tab_nonexistent not found" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestTabContext_RejectsRawCDPID(t *testing.T) {
	// TabContext should reject raw CDP target IDs (32-char hex)
	// These should never be accepted - only hash-format IDs work
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil)

	// Simulate a raw CDP target ID format
	rawCDPID := "A25658CE1BA82659EBE9C93C46CEE63A"

	_, _, err := tm.TabContext(rawCDPID)
	if err == nil {
		t.Error("TabContext should reject raw CDP target IDs")
	}
}

func TestCreateTab_ReturnsHashFormat(t *testing.T) {
	// Verify CreateTab returns hash-format IDs (tab_XXXXXXXX)
	// Note: This test can't actually create tabs without Chrome,
	// but we can test the ID format logic directly
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil)

	// CreateTab will fail (no browser), but we can test the idMgr
	if tm.idMgr == nil {
		t.Error("TabManager should have idMgr initialized")
	}

	// Test the ID generation format
	hashID := tm.idMgr.TabIDFromCDPTarget("A25658CE1BA82659EBE9C93C46CEE63A")
	if len(hashID) < 4 || hashID[:4] != "tab_" {
		t.Errorf("expected hash ID to start with 'tab_', got %s", hashID)
	}
	if len(hashID) != 12 { // tab_ + 8 hex chars
		t.Errorf("expected hash ID length 12, got %d (%s)", len(hashID), hashID)
	}
}

func TestTabContext_AcceptsRegisteredHashID(t *testing.T) {
	// TabContext should accept hash IDs that are properly registered
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil)

	// Manually register a tab entry (simulating what CreateTab does internally)
	hashID := "tab_abc12345"
	rawCDPID := "RAWCDPID123456789012345678901234"
	ctx := context.Background()

	tm.tabs[hashID] = &TabEntry{
		Ctx:   ctx,
		CDPID: rawCDPID,
	}

	// TabContext should find and return it
	returnedCtx, resolvedID, err := tm.TabContext(hashID)
	if err != nil {
		t.Errorf("TabContext should accept registered hash ID: %v", err)
	}
	if returnedCtx != ctx {
		t.Error("TabContext should return the registered context")
	}
	// resolvedID should be the raw CDP ID for internal consistency
	if resolvedID != rawCDPID {
		t.Errorf("resolvedID should be raw CDP ID, got %s", resolvedID)
	}
}

func TestCloseTab_PreventsLastTabClose(t *testing.T) {
	// CloseTab should fail when attempting to close the last remaining tab
	// This prevents Chrome from exiting and crashing the server
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil)

	// Without a valid browser context, ListTargets will fail
	// which triggers the guard at the start of CloseTab
	err := tm.CloseTab("tab_fake1234")
	if err == nil {
		t.Error("CloseTab should fail when ListTargets fails")
	}

	// The error should mention listing targets
	if err != nil && !strings.Contains(err.Error(), "list targets") {
		t.Errorf("expected error about list targets, got: %s", err.Error())
	}
}
