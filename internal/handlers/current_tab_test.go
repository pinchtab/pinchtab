package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
)

type scopedCurrentTabBridge struct {
	bridge.BridgeAPI
	tabs       map[string]context.Context
	requested  []string
	created    []string
	closed     []string
	globalTab  string
	ensureCall int
}

func newScopedCurrentTabBridge() *scopedCurrentTabBridge {
	return &scopedCurrentTabBridge{
		tabs: map[string]context.Context{
			"global":       context.Background(),
			"tab-agent":    context.Background(),
			"tab-session":  context.Background(),
			"tab-explicit": context.Background(),
		},
		globalTab: "global",
	}
}

func (b *scopedCurrentTabBridge) TabContext(tabID string) (context.Context, string, error) {
	if tabID == "" {
		tabID = b.globalTab
	}
	b.requested = append(b.requested, tabID)
	ctx, ok := b.tabs[tabID]
	if !ok {
		return nil, "", fmt.Errorf("tab not found")
	}
	return ctx, tabID, nil
}

func (b *scopedCurrentTabBridge) CreateTab(url string) (string, context.Context, context.CancelFunc, error) {
	b.created = append(b.created, url)
	tabID := fmt.Sprintf("tab-created-%d", len(b.created))
	ctx := context.Background()
	b.tabs[tabID] = ctx
	return tabID, ctx, func() {}, nil
}

func (b *scopedCurrentTabBridge) CloseTab(tabID string) error {
	b.closed = append(b.closed, tabID)
	delete(b.tabs, tabID)
	return nil
}

func (b *scopedCurrentTabBridge) FocusTab(tabID string) error {
	if _, ok := b.tabs[tabID]; !ok {
		return fmt.Errorf("tab not found")
	}
	return nil
}

func (b *scopedCurrentTabBridge) EnsureChrome(*config.RuntimeConfig) error {
	b.ensureCall++
	return nil
}

func (b *scopedCurrentTabBridge) RestartBrowser(*config.RuntimeConfig) error { return nil }

func (b *scopedCurrentTabBridge) ListTargets() ([]*target.Info, error) {
	return []*target.Info{
		{TargetID: target.ID("global"), Type: "page"},
		{TargetID: target.ID("tab-agent"), Type: "page"},
		{TargetID: target.ID("tab-session"), Type: "page"},
		{TargetID: target.ID("tab-explicit"), Type: "page"},
	}, nil
}

func (b *scopedCurrentTabBridge) AvailableActions() []string             { return nil }
func (b *scopedCurrentTabBridge) TabLockInfo(string) *bridge.LockInfo    { return nil }
func (b *scopedCurrentTabBridge) DeleteRefCache(string)                  {}
func (b *scopedCurrentTabBridge) NetworkMonitor() *bridge.NetworkMonitor { return nil }

func newScopedCurrentTabHandler() (*Handlers, *scopedCurrentTabBridge) {
	b := newScopedCurrentTabBridge()
	return New(b, &config.RuntimeConfig{}, nil, nil, nil), b
}

func TestScopedCurrentTabSessionBeatsAgentID(t *testing.T) {
	h, _ := newScopedCurrentTabHandler()

	agentReq := httptest.NewRequest("GET", "/text", nil)
	agentReq.Header.Set(activity.HeaderAgentID, "agent-1")
	h.setCurrentTabForRequest(agentReq, "tab-agent")

	sessionReq := httptest.NewRequest("GET", "/text", nil)
	sessionReq.Header.Set(activity.HeaderAgentID, "agent-1")
	sessionReq = session.WithSession(sessionReq, &session.Session{ID: "ses_1", AgentID: "agent-1"})
	h.setCurrentTabForRequest(sessionReq, "tab-session")

	_, got, err := h.tabContext(sessionReq, "")
	if err != nil {
		t.Fatalf("session tabContext error = %v", err)
	}
	if got != "tab-session" {
		t.Fatalf("session scoped tab = %q, want tab-session", got)
	}

	_, got, err = h.tabContext(agentReq, "")
	if err != nil {
		t.Fatalf("agent tabContext error = %v", err)
	}
	if got != "tab-agent" {
		t.Fatalf("agent scoped tab = %q, want tab-agent", got)
	}
}

func TestScopedCurrentTabDoesNotFallBackToGlobal(t *testing.T) {
	h, b := newScopedCurrentTabHandler()

	req := httptest.NewRequest("GET", "/text", nil)
	req.Header.Set(activity.HeaderAgentID, "agent-without-current")

	if _, _, err := h.tabContext(req, ""); err == nil {
		t.Fatal("expected missing scoped current tab to fail")
	}
	if len(b.requested) != 0 {
		t.Fatalf("bridge should not be asked for global tab, got requests %v", b.requested)
	}
}

// trustedSessionRequest builds a request that mimics a trusted-internal-proxy
// hop carrying a session id header. Public callers cannot fabricate this;
// the marker is set on the orchestrator → instance hop after auth.
func trustedSessionRequest(method, path, sessionID string, body []byte) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set(activity.HeaderPTSessionID, sessionID)
	return req.WithContext(MarkTrustedInternalProxy(req.Context()))
}

func TestExplicitTabUpdatesScopedCurrentTab(t *testing.T) {
	h, _ := newScopedCurrentTabHandler()

	req := trustedSessionRequest("GET", "/text", "ses_header", nil)

	if _, got, err := h.tabContext(req, "tab-explicit"); err != nil || got != "tab-explicit" {
		t.Fatalf("explicit tabContext = %q, %v; want tab-explicit, nil", got, err)
	}

	if _, got, err := h.tabContext(req, ""); err != nil || got != "tab-explicit" {
		t.Fatalf("scoped current after explicit = %q, %v; want tab-explicit, nil", got, err)
	}
}

func TestClearCurrentTabReferencesClearsAllScopesForTab(t *testing.T) {
	h, _ := newScopedCurrentTabHandler()

	sessionReq := trustedSessionRequest("GET", "/text", "ses_1", nil)
	agentReq := httptest.NewRequest("GET", "/text", nil)
	agentReq.Header.Set(activity.HeaderAgentID, "agent-1")

	h.setCurrentTabForRequest(sessionReq, "tab-explicit")
	h.setCurrentTabForRequest(agentReq, "tab-explicit")
	h.clearCurrentTabReferences("tab-explicit")

	if _, _, err := h.tabContext(sessionReq, ""); err == nil {
		t.Fatal("expected cleared session current tab to fail")
	}
	if _, _, err := h.tabContext(agentReq, ""); err == nil {
		t.Fatal("expected cleared agent current tab to fail")
	}
}

func TestNavigateUsesScopedCurrentTabWhenPresent(t *testing.T) {
	h, b := newScopedCurrentTabHandler()

	req := trustedSessionRequest("POST", "/navigate", "ses_1", []byte(`{"url":"about:blank"}`))
	h.setCurrentTabForRequest(req, "tab-session")

	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if len(b.created) != 0 {
		t.Fatalf("navigate should reuse scoped current tab, created tabs %v", b.created)
	}
	if len(b.requested) == 0 || b.requested[0] != "tab-session" {
		t.Fatalf("navigate requested tabs = %v, want first tab-session", b.requested)
	}
}

func TestNavigateCreatesTabWhenIdentifiedCallerHasNoCurrentTab(t *testing.T) {
	h, b := newScopedCurrentTabHandler()

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"url":"about:blank"}`)))
	req.Header.Set(activity.HeaderAgentID, "agent-without-current")

	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if len(b.created) != 1 {
		t.Fatalf("navigate should create a tab when scoped current is absent, created %v", b.created)
	}
}

func TestCurrentTabStore_LRUCapEvictsOldest(t *testing.T) {
	store := NewCurrentTabStore()
	store.now = deterministicClock()
	store.SetCap(2)

	scopes := []currentTabScope{
		scopedCurrentTab(currentTabScopeAgent, "a"),
		scopedCurrentTab(currentTabScopeAgent, "b"),
		scopedCurrentTab(currentTabScopeAgent, "c"),
	}
	for i, sc := range scopes {
		store.Set(sc, fmt.Sprintf("tab-%d", i))
	}

	// First entry must be evicted; last two survive.
	if _, ok := store.Get(scopes[0]); ok {
		t.Fatal("oldest entry should have been evicted by LRU cap")
	}
	if _, ok := store.Get(scopes[1]); !ok {
		t.Fatal("middle entry should remain")
	}
	if _, ok := store.Get(scopes[2]); !ok {
		t.Fatal("newest entry should remain")
	}
}

func TestCurrentTabStore_GetBumpsRecency(t *testing.T) {
	store := NewCurrentTabStore()
	store.now = deterministicClock()
	store.SetCap(2)

	a := scopedCurrentTab(currentTabScopeAgent, "a")
	b := scopedCurrentTab(currentTabScopeAgent, "b")
	c := scopedCurrentTab(currentTabScopeAgent, "c")

	store.Set(a, "tab-a")
	store.Set(b, "tab-b")
	// Touch 'a' so it's now the most recently used; b is the LRU.
	if _, ok := store.Get(a); !ok {
		t.Fatal("a should still be present")
	}
	store.Set(c, "tab-c") // forces eviction of LRU = b

	if _, ok := store.Get(b); ok {
		t.Fatal("b should have been evicted, not a")
	}
	if _, ok := store.Get(a); !ok {
		t.Fatal("a should have survived because Get bumped recency")
	}
	if _, ok := store.Get(c); !ok {
		t.Fatal("c should be present")
	}
}

func deterministicClock() func() time.Time {
	now := time.Unix(0, 0)
	return func() time.Time {
		now = now.Add(time.Nanosecond)
		return now
	}
}

// TestUntrustedSessionHeaderIgnored guards the spoof case: a public client
// who manages to set X-PinchTab-Session-Id without the trusted-internal
// marker must NOT have the header honored as identity.
func TestUntrustedSessionHeaderIgnored(t *testing.T) {
	h, _ := newScopedCurrentTabHandler()

	trusted := trustedSessionRequest("GET", "/text", "ses_owner", nil)
	h.setCurrentTabForRequest(trusted, "tab-explicit")

	// Untrusted request claims the same session id but lacks the marker.
	spoofed := httptest.NewRequest("GET", "/text", nil)
	spoofed.Header.Set(activity.HeaderPTSessionID, "ses_owner")

	if got, ok := h.scopedCurrentTabForRequest(spoofed); ok {
		t.Fatalf("untrusted header must not resolve to %q", got)
	}
}
