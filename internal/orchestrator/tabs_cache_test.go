package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestTabsCache_GetMissAndHit(t *testing.T) {
	c := NewTabsCache(time.Second, nil)
	if _, ok := c.Get("inst_a"); ok {
		t.Fatal("empty cache should miss")
	}
	c.Set("inst_a", []bridge.InstanceTab{{ID: "tab-1", InstanceID: "inst_a"}})
	got, ok := c.Get("inst_a")
	if !ok || len(got) != 1 || got[0].ID != "tab-1" {
		t.Fatalf("hit = %v / %#v", ok, got)
	}
}

func TestTabsCache_Expiry(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	c := NewTabsCache(500*time.Millisecond, func() time.Time { return clock })
	c.Set("inst_a", []bridge.InstanceTab{{ID: "tab-1"}})

	clock = t0.Add(200 * time.Millisecond)
	if _, ok := c.Get("inst_a"); !ok {
		t.Fatal("should still hit before expiry")
	}
	clock = t0.Add(700 * time.Millisecond)
	if _, ok := c.Get("inst_a"); ok {
		t.Fatal("should miss after expiry")
	}
}

func TestTabsCache_Invalidate(t *testing.T) {
	c := NewTabsCache(time.Second, nil)
	c.Set("inst_a", []bridge.InstanceTab{{ID: "tab-1"}})
	c.Invalidate("inst_a")
	if _, ok := c.Get("inst_a"); ok {
		t.Fatal("invalidate should drop entry")
	}
}

func TestTabsCache_InvalidateAll(t *testing.T) {
	c := NewTabsCache(time.Second, nil)
	c.Set("inst_a", []bridge.InstanceTab{{ID: "tab-1"}})
	c.Set("inst_b", []bridge.InstanceTab{{ID: "tab-2"}})
	c.InvalidateAll()
	if c.Len() != 0 {
		t.Fatalf("Len after InvalidateAll = %d", c.Len())
	}
}

func TestTabsCache_NilSafe(t *testing.T) {
	var c *TabsCache
	c.Set("a", nil)
	c.Invalidate("a")
	c.InvalidateAll()
	if _, ok := c.Get("a"); ok {
		t.Fatal("nil cache should always miss")
	}
	if c.Len() != 0 {
		t.Fatal("nil cache Len should be 0")
	}
}

func TestTabsCacheRequestAffectsTabs_PostShorthand(t *testing.T) {
	cases := map[string]bool{
		"/navigate":                         true,
		"/tab":                              true,
		"/close":                            true,
		"/reload":                           true,
		"/back":                             true,
		"/forward":                          true,
		"/instances/inst_a/tab":             true,
		"/instances/inst_a/tabs/X/navigate": true,
		"/text":                             false,
		"/snap":                             false,
	}
	for path, want := range cases {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		got := tabsCacheRequestAffectsTabs(req, nil)
		if got != want {
			t.Errorf("path %q affects = %v, want %v", path, got, want)
		}
	}
}

func TestTabsCacheRequestAffectsTabs_TabSubroutes(t *testing.T) {
	for _, path := range []string{
		"/tabs/X/close",
		"/tabs/X/navigate",
		"/tabs/X/reload",
		"/tabs/X/back",
		"/tabs/X/forward",
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if !tabsCacheRequestAffectsTabs(req, nil) {
			t.Errorf("expected %q to invalidate", path)
		}
	}
	for _, path := range []string{"/tabs/X/snapshot", "/tabs/X/text"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if tabsCacheRequestAffectsTabs(req, nil) {
			t.Errorf("expected %q NOT to invalidate", path)
		}
	}
}

func TestTabsCacheRequestAffectsTabs_GETIgnoredUnlessHeaderSurfaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	if tabsCacheRequestAffectsTabs(req, nil) {
		t.Fatal("plain GET should not invalidate")
	}

	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set(activity.HeaderPTTabID, "tab-just-changed")
	if !tabsCacheRequestAffectsTabs(req, resp) {
		t.Fatal("response carrying X-PinchTab-Tab-Id should invalidate")
	}
}

func TestOrchestrator_InstanceTabsCachedReusesEntry(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tabs" {
			calls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tabs":[{"id":"tab-1","url":"about:blank"}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	o.client = srv.Client()
	inst := &InstanceInternal{
		Instance: bridge.Instance{ID: "inst_a", Status: "running", URL: srv.URL},
		URL:      srv.URL,
		cmd:      &mockCmd{pid: 1, isAlive: true},
	}
	o.instances["inst_a"] = inst

	if _, err := o.instanceTabsCached(inst, false); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := o.instanceTabsCached(inst, false); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 backend call (cache hit on second), got %d", calls)
	}

	// fresh=true bypasses cache.
	if _, err := o.instanceTabsCached(inst, true); err != nil {
		t.Fatalf("fresh fetch: %v", err)
	}
	if calls != 2 {
		t.Fatalf("fresh=true should bypass, got %d backend calls", calls)
	}

	// Invalidate clears.
	o.tabsCache.Invalidate("inst_a")
	if _, err := o.instanceTabsCached(inst, false); err != nil {
		t.Fatalf("post-invalidate fetch: %v", err)
	}
	if calls != 3 {
		t.Fatalf("after invalidate, expected 3 backend calls, got %d", calls)
	}
}

func TestOrchestrator_InstanceStoppedClearsTabsCache(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{})
	o.tabsCache.Set("inst_a", []bridge.InstanceTab{{ID: "tab-1"}})

	o.EmitEvent("instance.stopped", &bridge.Instance{ID: "inst_a"})

	if _, ok := o.tabsCache.Get("inst_a"); ok {
		t.Fatal("instance.stopped should drop cached tabs for that instance")
	}
}
