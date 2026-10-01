package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const sessionTabsSecret = "internal-secret"

type sessionTabsFixture struct {
	t *testing.T
	b *bridge.Bridge
	h *Handlers
}

func newSessionTabsFixture(t *testing.T) *sessionTabsFixture {
	t.Helper()
	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	// Not t.TempDir: closing a tab saves state from a goroutine, which can still be
	// writing sessions.json when the testing package's RemoveAll runs.
	stateDir, err := os.MkdirTemp("", "session-tabs-state-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancelTimeout()
		_ = os.RemoveAll(stateDir)
	})
	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{ActionTimeout: 5 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: stateDir}
	b := browserBridge(ctx, cfg)
	b.RegisterTab("seed", ctx)
	return &sessionTabsFixture{t: t, b: b, h: New(b, cfg, nil, nil, nil)}
}

func (f *sessionTabsFixture) serve(handler http.HandlerFunc, path, body, sessionID string, trusted bool) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set(activity.HeaderPTSessionID, sessionID)
	}
	if trusted {
		req.Header.Set(InternalTokenHeader, sessionTabsSecret)
	}
	rec := httptest.NewRecorder()
	TrustedInternalProxyStripMiddleware(sessionTabsSecret)(handler).ServeHTTP(rec, req)
	return rec
}

func (f *sessionTabsFixture) newTab(sessionID string, trusted bool) string {
	f.t.Helper()
	rec := f.serve(f.h.HandleTab, "/tab", `{"action":"new"}`, sessionID, trusted)
	var out struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.TabID == "" {
		f.t.Fatalf("new tab for %q answered %d: %s", sessionID, rec.Code, rec.Body.String())
	}
	return out.TabID
}

func (f *sessionTabsFixture) closeFor(sessionID string) sessionTabsCloseResult {
	f.t.Helper()
	rec := f.serve(f.h.HandleSessionTabsClose, SessionTabsClosePath, "", sessionID, true)
	if rec.Code != http.StatusOK {
		f.t.Fatalf("close for %q answered %d: %s", sessionID, rec.Code, rec.Body.String())
	}
	var out sessionTabsCloseResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *sessionTabsFixture) open(tabID string) bool {
	_, _, err := f.b.TabContext(tabID)
	return err == nil
}

func keptIDs(kept []keptSessionTab) []string {
	ids := make([]string, 0, len(kept))
	for _, k := range kept {
		ids = append(ids, k.TabID)
	}
	sort.Strings(ids)
	return ids
}

func TestAnEndedSessionClosesOnlyTheTabsItCreatedAndNobodyElseUsed(t *testing.T) {
	f := newSessionTabsFixture(t)
	own := f.newTab("sess_s", true)
	shared := f.newTab("sess_s", true)
	paused := f.newTab("sess_s", true)
	locked := f.newTab("sess_s", true)
	other := f.newTab("sess_t", true)
	global := f.newTab("", false)

	if rec := f.serve(f.h.HandleTab, "/tab", `{"action":"focus","tabId":"`+shared+`"}`, "sess_t", true); rec.Code != http.StatusOK {
		t.Fatalf("focus by the second session answered %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.b.SetTabHandoff(paused, "captcha", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := f.b.Lock(locked, "agent-s", time.Minute); err != nil {
		t.Fatal(err)
	}

	logs := captureSlog(t)
	first := f.closeFor("sess_s")
	if len(first.Closed) != 1 || first.Closed[0] != own {
		t.Fatalf("closed = %v, want only %s", first.Closed, own)
	}
	wantKept := []string{locked, paused}
	sort.Strings(wantKept)
	if got := keptIDs(first.Kept); len(got) != 2 || got[0] != wantKept[0] || got[1] != wantKept[1] {
		t.Fatalf("kept = %+v, want the paused and locked tabs %v", first.Kept, wantKept)
	}
	for _, id := range wantKept {
		if !strings.Contains(logs.String(), "tabId="+id) {
			t.Errorf("the kept tab %s was not logged: %s", id, logs.String())
		}
	}
	if f.open(own) {
		t.Fatalf("the ended session's own tab %s is still open", own)
	}
	for name, id := range map[string]string{"used by another session": shared, "paused": paused, "locked": locked, "another session's": other, "global": global} {
		if !f.open(id) {
			t.Errorf("the %s tab %s was closed", name, id)
		}
	}

	second := f.closeFor("sess_s")
	if len(second.Closed) != 0 {
		t.Fatalf("a second end of the same session closed %v, want nothing", second.Closed)
	}
}

func TestTheSessionTabsCloseRouteIsUnroutedOffTheTrustedHop(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, SessionTabsClosePath, nil)
	req.Header.Set(activity.HeaderPTSessionID, "sess_s")
	rec := httptest.NewRecorder()
	TrustedInternalProxyStripMiddleware(sessionTabsSecret)(http.HandlerFunc(h.HandleSessionTabsClose)).ServeHTTP(rec, req)
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusNotFound || body.Code != "not_found" {
		t.Fatalf("an untrusted caller got %d %q: %s, want the unrouted 404", rec.Code, body.Code, rec.Body.String())
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureSlog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return buf
}

func TestAPopupAClickOfTheSessionOpensIsClosedWithTheSession(t *testing.T) {
	f := newSessionTabsFixture(t)
	opener := f.newTab("sess_s", true)
	ctx, _, err := f.b.TabContext(opener)
	if err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerHTML = '<a id="pop" href="about:blank" target="_blank">pop</a>'`, nil)); err != nil {
		t.Fatal(err)
	}
	rec := f.serve(f.h.HandleAction, "/action", `{"kind":"click","selector":"#pop","tabId":"`+opener+`"}`, "sess_s", true)
	var out struct {
		Result map[string]any `json:"result"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	popup, _ := out.Result["switchedToTab"].(string)
	if popup == "" {
		t.Fatalf("the click opened no adopted popup (%d): %s", rec.Code, rec.Body.String())
	}
	closed := f.closeFor("sess_s").Closed
	sort.Strings(closed)
	want := []string{opener, popup}
	sort.Strings(want)
	if len(closed) != 2 || closed[0] != want[0] || closed[1] != want[1] {
		t.Fatalf("closed = %v, want the opener and its popup %v", closed, want)
	}
}
