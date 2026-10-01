package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/session"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func newFreezeIdleFixture(t *testing.T, delay time.Duration) (*bridge.Bridge, *Handlers) {
	t.Helper()
	tabCtx := testbrowser.Tab(t)
	cfg := &config.RuntimeConfig{TabLifecyclePolicy: "freeze_idle", TabCloseDelay: delay, ActionTimeout: 10 * time.Second, AllowScreencast: true, StateDir: t.TempDir()}
	b := browserBridge(tabCtx, cfg)
	b.RegisterTab("tabA", tabCtx)
	return b, New(b, cfg, nil, nil, nil)
}

func TestATabResolvedForARequestStaysAwakeUntilTheRequestEnds(t *testing.T) {
	b, h := newFreezeIdleFixture(t, 20*time.Millisecond)

	reqCtx, endRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/snapshot", nil).WithContext(reqCtx)
	if _, _, err := h.tabContext(req, "tabA"); err != nil {
		t.Fatalf("resolve tab: %v", err)
	}

	time.Sleep(80 * time.Millisecond)
	if b.TabFrozen("tabA") {
		t.Fatal("tab froze while its request was still running")
	}

	endRequest()
	deadline := time.Now().Add(time.Second)
	for !b.TabFrozen("tabA") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !b.TabFrozen("tabA") {
		t.Fatal("tab never froze after its request ended")
	}
}

func TestAFailedUnfreezeIsARetryable503AndKeepsTheSessionsCurrentTab(t *testing.T) {
	for _, route := range []struct {
		name    string
		request func() *http.Request
		retries bool
	}{
		{"text", func() *http.Request { return httptest.NewRequest(http.MethodGet, "/text", nil) }, true},
		{"screencast", func() *http.Request { return httptest.NewRequest(http.MethodGet, "/screencast?tabId=tabA", nil) }, false},
		{"record start", func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/record/start", strings.NewReader(`{"tabId":"tabA"}`))
		}, false},
	} {
		t.Run(route.name, func(t *testing.T) {
			b, h := newFreezeIdleFixture(t, 30*time.Millisecond)
			var failThaw atomic.Bool
			failThaw.Store(true)
			b.SetLifecycleWriterForTests(func(_ context.Context, frozen bool) error {
				if !frozen && failThaw.Swap(false) {
					return errors.New("renderer busy")
				}
				return nil
			})
			scope := scopedCurrentTab(currentTabScopeSession, "ses_frozen")
			h.CurrentTabs.Set(scope, "tabA")
			b.ScheduleIdleLifecycle("tabA")
			deadline := time.Now().Add(time.Second)
			for !b.TabFrozen("tabA") && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			if !b.TabFrozen("tabA") {
				t.Fatal("tab never froze")
			}
			mux := http.NewServeMux()
			h.RegisterRoutes(mux, nil)
			serve := func() *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, session.WithSession(route.request(), &session.Session{ID: "ses_frozen"}))
				return w
			}

			first := serve()
			var body struct {
				Code      string `json:"code"`
				Retryable bool   `json:"retryable"`
			}
			_ = json.Unmarshal(first.Body.Bytes(), &body)
			if first.Code != http.StatusServiceUnavailable || body.Code != "tab_unfreeze_failed" || !body.Retryable {
				t.Fatalf("failed unfreeze answered %d %s, want 503 tab_unfreeze_failed retryable", first.Code, first.Body.String())
			}
			if got, ok := h.CurrentTabs.Get(scope); !ok || got != "tabA" {
				t.Fatalf("session current tab = %q %v after a failed unfreeze, want tabA kept", got, ok)
			}
			if !route.retries {
				return
			}
			if second := serve(); second.Code != http.StatusOK {
				t.Fatalf("retry after the renderer recovered answered %d %s, want 200", second.Code, second.Body.String())
			}
		})
	}
}
