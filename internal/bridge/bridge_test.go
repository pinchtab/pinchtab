package bridge

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
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
	// This is the case for remote allocators where the browser context is
	// established lazily.
	cfg := &config.RuntimeConfig{}

	// Use context.TODO() instead of nil to avoid lint warnings
	ctx := context.TODO()
	tm := NewTabManager(ctx, cfg, nil, nil, nil)
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
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)

	// Try to get context for a non-existent tab
	_, _, err := tm.TabContext("tab_nonexistent")
	if err == nil {
		t.Error("TabContext should reject unknown tab IDs")
	}
	if err.Error() != "tab tab_nonexistent not found" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestTabContext_RejectsUnknownRawCDPID(t *testing.T) {
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)

	rawCDPID := "A25658CE1BA82659EBE9C93C46CEE63A"

	_, _, err := tm.TabContext(rawCDPID)
	if err == nil {
		t.Error("TabContext should reject unknown raw CDP target IDs")
	}
}

func TestQuietStealthObservers(t *testing.T) {
	if (&Bridge{Config: &config.RuntimeConfig{StealthLevel: "full"}}).quietStealthObservers() != true {
		t.Fatal("expected full stealth to suppress nonessential observers")
	}
	if (&Bridge{Config: &config.RuntimeConfig{StealthLevel: "medium"}}).quietStealthObservers() {
		t.Fatal("did not expect medium stealth to suppress observers")
	}
}

func TestCreateTab_ReturnsRawCDPID(t *testing.T) {
	// Verify TabIDFromCDPTarget returns raw CDP ID (no prefix)
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)

	if tm.idMgr == nil {
		t.Error("TabManager should have idMgr initialized")
	}

	rawCDP := "A25658CE1BA82659EBE9C93C46CEE63A"
	tabID := tm.idMgr.TabIDFromCDPTarget(rawCDP)

	if tabID != rawCDP {
		t.Errorf("expected %s, got %s", rawCDP, tabID)
	}
}

func TestTabContext_AcceptsRegisteredID(t *testing.T) {
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)

	rawCDPID := "RAWCDPID123456789012345678901234"
	ctx := context.Background()

	tm.tabs[rawCDPID] = &TabEntry{
		Ctx:   ctx,
		CDPID: rawCDPID,
	}

	returnedCtx, resolvedID, err := tm.TabContext(rawCDPID)
	if err != nil {
		t.Errorf("TabContext should accept registered ID: %v", err)
	}
	if returnedCtx != ctx {
		t.Error("TabContext should return the registered context")
	}
	if resolvedID != rawCDPID {
		t.Errorf("resolvedID should be tab ID, got %s", resolvedID)
	}
}

func TestTabContext_EmptyID_UsesCurrentTrackedTab(t *testing.T) {
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)

	ctx := context.Background()
	tabID := "SOMECDPID"

	tm.mu.Lock()
	tm.tabs[tabID] = &TabEntry{Ctx: ctx, CDPID: tabID}
	tm.currentTab = tabID
	tm.mu.Unlock()

	returnedCtx, resolvedID, err := tm.TabContext("")
	if err != nil {
		t.Fatalf("TabContext(\"\") should resolve to current tab: %v", err)
	}
	if returnedCtx != ctx {
		t.Error("should return the current tab's context")
	}
	if resolvedID != tabID {
		t.Errorf("expected %s, got %s", tabID, resolvedID)
	}
}

func TestCloseTab_PreventsLastTabClose(t *testing.T) {
	// CloseTab should fail when attempting to close the last remaining tab
	// This prevents Chrome from exiting and crashing the server
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)

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

func TestShouldBlockPopupTarget(t *testing.T) {
	tests := []struct {
		name string
		info *target.Info
		want bool
	}{
		{name: "nil target", info: nil, want: false},
		{name: "top level page", info: &target.Info{Type: TargetTypePage}, want: false},
		{name: "non-page target with opener", info: &target.Info{Type: "service_worker", OpenerID: target.ID("opener")}, want: false},
		{name: "page popup", info: &target.Info{Type: TargetTypePage, OpenerID: target.ID("opener")}, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldBlockPopupTarget(tc.info); got != tc.want {
				t.Fatalf("shouldBlockPopupTarget(%+v) = %v, want %v", tc.info, got, tc.want)
			}
		})
	}
}

func TestEvaluateTabPolicy(t *testing.T) {
	cfg := config.IDPIConfig{
		Enabled:        true,
		AllowedDomains: []string{"example.com"},
		StrictMode:     true,
	}

	allowed := EvaluateTabPolicy("https://example.com/path", cfg)
	if allowed.Threat || allowed.Blocked {
		t.Fatalf("expected allowed domain to pass, got %+v", allowed)
	}

	blocked := EvaluateTabPolicy("https://evil.example.net/path", cfg)
	if !blocked.Threat || !blocked.Blocked {
		t.Fatalf("expected blocked domain to fail, got %+v", blocked)
	}
	if blocked.CurrentURL == "" || blocked.UpdatedAt.IsZero() {
		t.Fatalf("expected policy state metadata to be populated, got %+v", blocked)
	}
}

func TestTabManagerStoresTabPolicyState(t *testing.T) {
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, nil, nil)
	tm.tabs["tab1"] = &TabEntry{Ctx: context.Background()}

	state := TabPolicyState{
		CurrentURL: "https://evil.example.net/path",
		Threat:     true,
		Blocked:    true,
		Reason:     "blocked",
		UpdatedAt:  time.Now(),
	}
	tm.SetTabPolicyState("tab1", state)

	got, ok := tm.GetTabPolicyState("tab1")
	if !ok {
		t.Fatal("expected stored policy state")
	}
	if got.CurrentURL != state.CurrentURL || got.Blocked != state.Blocked || got.Reason != state.Reason {
		t.Fatalf("stored policy state mismatch: got %+v want %+v", got, state)
	}
}

func TestPurgeTrackedTabStateByTargetID(t *testing.T) {
	logStore := NewConsoleLogStore(10)
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{}, nil, logStore, nil)
	dm := NewDialogManager()
	tm.SetDialogManager(dm)

	tabID := "public-tab-id"
	cdpID := "RAWCDPID123"
	tm.tabs[tabID] = &TabEntry{Ctx: context.Background(), CDPID: cdpID}
	tm.snapshots[tabID] = &RefCache{Refs: map[string]int64{"e0": 1}}
	tm.accessed[tabID] = true
	tm.currentTab = tabID
	dm.SetPending(tabID, &DialogState{Type: "alert", Message: "secret"})
	logStore.AddConsoleLog(cdpID, LogEntry{Level: "log", Message: "secret"})
	logStore.AddErrorLog(cdpID, ErrorEntry{Message: "secret"})

	if ok := tm.purgeTrackedTabStateByTargetID(cdpID); !ok {
		t.Fatal("expected tab cleanup to succeed")
	}
	if _, ok := tm.tabs[tabID]; ok {
		t.Fatal("expected tracked tab to be removed")
	}
	if _, ok := tm.snapshots[tabID]; ok {
		t.Fatal("expected snapshot cache to be removed")
	}
	if tm.accessed[tabID] {
		t.Fatal("expected accessed entry to be removed")
	}
	if tm.currentTab != "" {
		t.Fatalf("expected current tab to be cleared, got %q", tm.currentTab)
	}
	if dm.GetPending(tabID) != nil {
		t.Fatal("expected pending dialog to be cleared")
	}
	if logs := logStore.GetConsoleLogs(cdpID, 0); logs != nil {
		t.Fatalf("expected console logs to be removed, got %v", logs)
	}
	if errs := logStore.GetErrorLogs(cdpID, 0); errs != nil {
		t.Fatalf("expected error logs to be removed, got %v", errs)
	}
}
