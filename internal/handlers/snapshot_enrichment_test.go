package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const enrichmentFixtureHTML = `<!doctype html><html><head><title>Signup</title></head><body>
<form>
  <label for="email">Email</label>
  <input id="email" type="text" placeholder="you@example.com" data-testid="signup-email">
  <button type="submit" data-testid="checkout-submit">Continue</button>
</form>
</body></html>`

func newEnrichmentFixture(t *testing.T) (*Handlers, string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(enrichmentFixtureHTML))
	}))
	t.Cleanup(server.Close)

	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)

	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL), chromedp.WaitVisible("#email", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{ActionTimeout: 10 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := browserBridge(ctx, cfg)
	b.RegisterTab("tab-enrich", ctx)
	return New(b, cfg, nil, nil, nil), "tab-enrich"
}

func snapshotFor(t *testing.T, h *Handlers, tabID, format string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/snapshot?tabId="+tabID+"&format="+format, nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot (%s): status %d body=%s", format, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func enrichedNodes(t *testing.T, h *Handlers, tabID string) int {
	t.Helper()
	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil || len(cache.Nodes) == 0 {
		t.Fatalf("no snapshot cache for %s, so nothing here observes the enrichment either way", tabID)
	}
	enriched := 0
	for _, node := range cache.Nodes {
		if node.Tag != "" || node.TestID != "" || node.Placeholder != "" {
			enriched++
		}
	}
	return enriched
}

func TestACompactSnapshotSkipsEnrichmentAndFindStillResolves(t *testing.T) {
	h, tabID := newEnrichmentFixture(t)

	body := snapshotFor(t, h, tabID, "compact")
	if !strings.Contains(body, "textbox") {
		t.Fatalf("the compact snapshot did not describe the form, so this proves nothing: %s", body)
	}
	if got := enrichedNodes(t, h, tabID); got != 0 {
		t.Errorf("%d cached nodes carry DOM metadata after a compact snapshot; the format cannot render those fields, so the round trip that filled them was spent on nothing", got)
	}

	wantRef := refOf(t, h, tabID, "textbox", "Email")
	findReq := httptest.NewRequest("POST", "/find", strings.NewReader(`{"tabId":"`+tabID+`","query":"you@example.com"}`))
	findRes := httptest.NewRecorder()
	h.HandleFind(findRes, findReq)
	if findRes.Code != http.StatusOK {
		t.Fatalf("find after a compact snapshot: status %d body=%s", findRes.Code, findRes.Body.String())
	}

	var found struct {
		BestRef string `json:"best_ref"`
	}
	if err := json.Unmarshal(findRes.Body.Bytes(), &found); err != nil {
		t.Fatalf("unreadable find response: %v\n%s", err, findRes.Body.String())
	}
	if found.BestRef != wantRef {
		t.Errorf("find matched %q, want %q — the query is the field's placeholder, which only the DOM enrichment supplies, so a miss means the matcher scored nodes it never reached: %s",
			found.BestRef, wantRef, findRes.Body.String())
	}
	if got := enrichedNodes(t, h, tabID); got == 0 {
		t.Errorf("find left the cached nodes unenriched, so the matcher scored them without the fields it matches on")
	}
}

func TestAJSONSnapshotStillCarriesTheMatcherFields(t *testing.T) {
	h, tabID := newEnrichmentFixture(t)

	body := snapshotFor(t, h, tabID, "json")
	for _, field := range []string{`"tag"`, `"testid"`, `"placeholder"`} {
		if !strings.Contains(body, field) {
			t.Errorf("the json snapshot no longer carries %s; skipping enrichment for the cheap formats must not narrow the one that can render it:\n%s", field, body)
		}
	}
	if got := enrichedNodes(t, h, tabID); got == 0 {
		t.Errorf("a json snapshot left every cached node unenriched")
	}
}

func refOf(t *testing.T, h *Handlers, tabID, role, name string) string {
	t.Helper()
	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil {
		t.Fatal("no snapshot cache to read refs from")
	}
	for _, node := range cache.Nodes {
		if node.Role == role && node.Name == name {
			return node.Ref
		}
	}
	t.Fatalf("no %s named %q in the snapshot: %+v", role, name, cache.Nodes)
	return ""
}

type paramsCapturingBridge struct {
	*mockBridge
	params []bridge.ContentParams
}

func (b *paramsCapturingBridge) Snapshot(_ context.Context, _ string, _ string, params bridge.ContentParams) (*bridge.SnapshotResult, error) {
	b.params = append(b.params, params)
	return &bridge.SnapshotResult{
		Nodes: []bridge.A11yNode{{Ref: "e0", Role: "button", Name: "Buy"}},
		URL:   "http://127.0.0.1:1/fixture",
		Title: "Fixture",
	}, nil
}

func TestTheDelegatedSnapshotAsksForMetadataOnlyWhenTheFormatCarriesIt(t *testing.T) {
	for _, tc := range []struct {
		format   string
		wantSkip bool
	}{
		{format: "compact", wantSkip: true},
		{format: "text", wantSkip: true},
		{format: "json", wantSkip: false},
		{format: "yaml", wantSkip: false},
	} {
		t.Run(tc.format, func(t *testing.T) {
			stub := &paramsCapturingBridge{mockBridge: &mockBridge{}}
			h := New(stub, &config.RuntimeConfig{
				ActionTimeout:     5 * time.Second,
				DefaultBrowser:    config.BrowserGhostChrome,
				BrowsersAvailable: []string{config.BrowserGhostChrome},
				StateDir:          t.TempDir(),
			}, nil, nil, nil)

			req := httptest.NewRequest("GET", "/snapshot?tabId=tab1&format="+tc.format, nil)
			w := httptest.NewRecorder()
			h.HandleSnapshot(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("snapshot (%s): status %d body=%s", tc.format, w.Code, w.Body.String())
			}
			if len(stub.params) != 1 {
				t.Fatalf("the delegated path ran %d times, so this case does not observe what it asks for", len(stub.params))
			}
			if got := stub.params[0].SkipMetadata; got != tc.wantSkip {
				t.Errorf("format %s asked the bridge for SkipMetadata=%v, want %v; the delegated path pays the same per-node round trip the inline one does",
					tc.format, got, tc.wantSkip)
			}
		})
	}
}
