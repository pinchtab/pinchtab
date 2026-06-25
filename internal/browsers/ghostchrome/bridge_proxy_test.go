package ghostchrome

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/staticfetch"
)

type mockChromeBridge struct {
	mu               sync.Mutex
	tabs             map[string]bool
	failTab          bool
	createTabURLs    []string
	lastCreatedTabID string
	executeResult    map[string]any
	executeErr       error
	availableActions []string
}

func newMockChromeBridge() *mockChromeBridge {
	return &mockChromeBridge{
		tabs: map[string]bool{"chrome-tab-1": true},
	}
}

func (m *mockChromeBridge) TabContext(tabID string) (context.Context, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failTab {
		return nil, "", fmt.Errorf("tab not found")
	}
	if m.tabs[tabID] {
		return context.Background(), tabID, nil
	}
	return nil, "", fmt.Errorf("tab %q not found", tabID)
}

func (m *mockChromeBridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createTabURLs = append(m.createTabURLs, url)
	tabID := fmt.Sprintf("chrome-esc-%d", len(m.createTabURLs))
	m.lastCreatedTabID = tabID
	m.tabs[tabID] = true
	ctx, cancel := context.WithCancel(context.Background())
	return tabID, ctx, cancel, nil
}

func (m *mockChromeBridge) ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error) {
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	if m.executeResult != nil {
		return m.executeResult, nil
	}
	return map[string]any{"chrome": true}, nil
}

func (m *mockChromeBridge) AvailableActions() []string {
	if m.availableActions != nil {
		return m.availableActions
	}
	return []string{ActionClick, ActionType, ActionPress}
}

type ensureBrowserTracker struct {
	calls int
	err   error
}

func (t *ensureBrowserTracker) fn() func() error {
	return func() error {
		t.calls++
		return t.err
	}
}

func TestBridgeProxy_TabContext_ChromeTabPassthrough(t *testing.T) {
	mb := newMockChromeBridge()
	ec := &ensureBrowserTracker{}
	proxy := NewBridgeProxy(mb, staticfetch.NewBrowser(), ec.fn())

	ctx, resolved, err := proxy.TabContext("chrome-tab-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if resolved != "chrome-tab-1" {
		t.Fatalf("expected resolved = %q, got %q", "chrome-tab-1", resolved)
	}
	if ec.calls != 0 {
		t.Fatalf("expected 0 EnsureBrowser calls, got %d", ec.calls)
	}
	if len(mb.createTabURLs) != 0 {
		t.Fatalf("expected 0 CreateTab calls, got %d", len(mb.createTabURLs))
	}
}

func TestBridgeProxy_TabContext_LiteTabEscalates(t *testing.T) {
	mb := newMockChromeBridge()
	ec := &ensureBrowserTracker{}
	lite := staticfetch.NewBrowser()

	ts := startTestHTTPServer(t)
	defer ts.Close()

	navResult, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	liteTabID := navResult.TabID

	proxy := NewBridgeProxy(mb, lite, ec.fn())

	ctx, resolved, err := proxy.TabContext(liteTabID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ec.calls != 1 {
		t.Fatalf("expected 1 EnsureBrowser call, got %d", ec.calls)
	}
	if len(mb.createTabURLs) != 1 {
		t.Fatalf("expected 1 CreateTab call, got %d", len(mb.createTabURLs))
	}
	if mb.createTabURLs[0] != ts.URL {
		t.Fatalf("CreateTab URL = %q, want %q", mb.createTabURLs[0], ts.URL)
	}
	if resolved != mb.lastCreatedTabID {
		t.Fatalf("resolved = %q, want %q", resolved, mb.lastCreatedTabID)
	}
}

func TestBridgeProxy_TabContext_CachedEscalation(t *testing.T) {
	mb := newMockChromeBridge()
	ec := &ensureBrowserTracker{}
	lite := staticfetch.NewBrowser()

	ts := startTestHTTPServer(t)
	defer ts.Close()

	navResult, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	liteTabID := navResult.TabID

	proxy := NewBridgeProxy(mb, lite, ec.fn())

	_, _, err = proxy.TabContext(liteTabID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if ec.calls != 1 {
		t.Fatalf("expected 1 EnsureBrowser call, got %d", ec.calls)
	}

	_, resolved, err := proxy.TabContext(liteTabID)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if ec.calls != 1 {
		t.Fatalf("expected still 1 EnsureBrowser call, got %d", ec.calls)
	}
	if len(mb.createTabURLs) != 1 {
		t.Fatalf("expected still 1 CreateTab call, got %d", len(mb.createTabURLs))
	}
	if resolved != mb.lastCreatedTabID {
		t.Fatalf("resolved = %q, want %q", resolved, mb.lastCreatedTabID)
	}
}

func TestBridgeProxy_TabContext_StaleCachedTab(t *testing.T) {
	mb := newMockChromeBridge()
	ec := &ensureBrowserTracker{}
	lite := staticfetch.NewBrowser()

	ts := startTestHTTPServer(t)
	defer ts.Close()

	navResult, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	liteTabID := navResult.TabID

	proxy := NewBridgeProxy(mb, lite, ec.fn())

	_, _, err = proxy.TabContext(liteTabID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	firstChromeTab := mb.lastCreatedTabID

	delete(mb.tabs, firstChromeTab)

	_, resolved, err := proxy.TabContext(liteTabID)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if ec.calls != 2 {
		t.Fatalf("expected 2 EnsureBrowser calls, got %d", ec.calls)
	}
	if len(mb.createTabURLs) != 2 {
		t.Fatalf("expected 2 CreateTab calls, got %d", len(mb.createTabURLs))
	}
	if resolved == firstChromeTab {
		t.Fatal("resolved should be a new Chrome tab, not the stale one")
	}
}

func TestBridgeProxy_TabContext_ConcurrentEscalation(t *testing.T) {
	mb := newMockChromeBridge()
	ec := &ensureBrowserTracker{}
	lite := staticfetch.NewBrowser()

	ts := startTestHTTPServer(t)
	defer ts.Close()

	navResult, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	liteTabID := navResult.TabID

	proxy := NewBridgeProxy(mb, lite, ec.fn())

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	resolvedIDs := make([]string, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, resolved, err := proxy.TabContext(liteTabID)
			resolvedIDs[idx] = resolved
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, err)
		}
	}

	first := resolvedIDs[0]
	for i, id := range resolvedIDs[1:] {
		if id != first {
			t.Fatalf("goroutine %d resolved to %q, want %q (same as goroutine 0)", i+1, id, first)
		}
	}

	if len(mb.createTabURLs) != 1 {
		t.Fatalf("expected 1 CreateTab call, got %d", len(mb.createTabURLs))
	}
}

func TestBridgeProxy_ExecuteAction_ClickWithRef(t *testing.T) {
	mb := newMockChromeBridge()
	lite := staticfetch.NewBrowser()

	ts := startTestHTTPServer(t)
	defer ts.Close()

	navResult, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_, err = lite.Snapshot(context.Background(), navResult.TabID, "")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	proxy := NewBridgeProxy(mb, lite, nil)

	snap, _ := lite.Snapshot(context.Background(), navResult.TabID, "interactive")
	if len(snap.Nodes) == 0 {
		t.Skip("no interactive nodes in test page")
	}
	ref := snap.Nodes[0].Ref

	result, err := proxy.ExecuteAction(context.Background(), ActionClick, ActionRequest{
		TabID: navResult.TabID,
		Ref:   ref,
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction click: %v", err)
	}
	if result["clicked"] != true {
		t.Fatalf("expected clicked=true, got %v", result)
	}
	if _, has := result["chrome"]; has {
		t.Fatal("expected static browser to handle click, but went to Chrome")
	}
}

func TestBridgeProxy_ExecuteAction_ClickNoRef(t *testing.T) {
	mb := newMockChromeBridge()
	lite := staticfetch.NewBrowser()
	proxy := NewBridgeProxy(mb, lite, nil)

	result, err := proxy.ExecuteAction(context.Background(), ActionClick, ActionRequest{
		TabID: "chrome-tab-1",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction click: %v", err)
	}
	if result["chrome"] != true {
		t.Fatalf("expected chrome=true, got %v", result)
	}
}

func TestBridgeProxy_ExecuteAction_UnsupportedKind(t *testing.T) {
	mb := newMockChromeBridge()
	lite := staticfetch.NewBrowser()
	proxy := NewBridgeProxy(mb, lite, nil)

	result, err := proxy.ExecuteAction(context.Background(), ActionPress, ActionRequest{
		TabID: "chrome-tab-1",
		Ref:   "e5",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction press: %v", err)
	}
	if result["chrome"] != true {
		t.Fatalf("expected chrome=true, got %v", result)
	}
}

func TestBridgeProxy_NilLite_Passthrough(t *testing.T) {
	mb := newMockChromeBridge()
	proxy := NewBridgeProxy(mb, nil, nil)

	_, resolved, err := proxy.TabContext("chrome-tab-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resolved != "chrome-tab-1" {
		t.Fatalf("resolved = %q, want %q", "chrome-tab-1", resolved)
	}

	_, _, err = proxy.TabContext("unknown-tab")
	if err == nil {
		t.Fatal("expected error for unknown tab with nil lite, got nil")
	}

	result, err := proxy.ExecuteAction(context.Background(), ActionClick, ActionRequest{
		TabID: "chrome-tab-1",
		Ref:   "e5",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if result["chrome"] != true {
		t.Fatalf("expected chrome=true, got %v", result)
	}

	actions := proxy.AvailableActions()
	if len(actions) != 3 {
		t.Fatalf("expected 3 actions, got %d: %v", len(actions), actions)
	}

	_, found := proxy.TabURL("any")
	if found {
		t.Fatal("expected TabURL=false with nil lite")
	}

	if proxy.StaticBrowser() != nil {
		t.Fatal("expected StaticBrowser()=nil with nil lite")
	}
}

func TestBridgeProxy_AvailableActions_MergesStaticAndChrome(t *testing.T) {
	mb := newMockChromeBridge()
	mb.availableActions = []string{ActionPress}

	lite := staticfetch.NewBrowser()
	proxy := NewBridgeProxy(mb, lite, nil)

	actions := proxy.AvailableActions()
	actionSet := make(map[string]bool, len(actions))
	for _, a := range actions {
		actionSet[a] = true
	}

	for _, want := range []string{ActionPress, ActionClick, ActionType} {
		if !actionSet[want] {
			t.Errorf("expected %q in available actions, got %v", want, actions)
		}
	}
}

func startTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
  <h1>Hello</h1>
  <button id="btn1">Click Me</button>
  <input type="text" id="input1" placeholder="Type here">
  <a href="/page2">Link</a>
</body>
</html>`)
	}))
}

// L4 regression: the merged action set is map-built; its order must be
// deterministic because handlers interpolate it into error messages.
func TestAvailableActions_DeterministicSortedOrder(t *testing.T) {
	mb := newMockChromeBridge()
	mb.availableActions = []string{ActionScroll, ActionClick, ActionPress}
	lite := staticfetch.NewBrowser()
	defer func() { _ = lite.Close() }()
	p := NewBridgeProxy(mb, lite, nil)

	first := p.AvailableActions()
	if !sort.StringsAreSorted(first) {
		t.Fatalf("actions not sorted: %v", first)
	}
	second := p.AvailableActions()
	if len(first) != len(second) {
		t.Fatalf("lengths differ across calls: %v vs %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("order differs across calls: %v vs %v", first, second)
		}
	}
}
