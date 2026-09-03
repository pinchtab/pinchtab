package activity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/authn"
)

type captureRecorder struct {
	events []Event
}

func (c *captureRecorder) Enabled() bool { return true }

func (c *captureRecorder) Record(evt Event) error {
	c.events = append(c.events, evt)
	return nil
}

func (c *captureRecorder) Query(Filter) ([]Event, error) {
	return append([]Event(nil), c.events...), nil
}

func TestSanitizeActivityURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "drops query fragment and credentials",
			raw:  "https://user:pass@App.EXAMPLE.com:8443/callback?code=secret#done",
			want: "https://app.example.com:8443/callback",
		},
		{
			name: "normalizes bare hostname",
			raw:  "pinchtab.com/reset?token=secret",
			want: "https://pinchtab.com/reset",
		},
		{
			name: "keeps non-network scheme without fragment",
			raw:  "about:blank#frag",
			want: "about:blank",
		},
		{
			name: "rejects malformed",
			raw:  "://bad-url",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeActivityURL(tt.raw); got != tt.want {
				t.Fatalf("sanitizeActivityURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMiddlewareSanitizesInitialURL(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/navigate?url="+url.QueryEscape("pinchtab.com/reset?token=secret#done"), nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].URL; got != "https://pinchtab.com/reset" {
		t.Fatalf("event.URL = %q, want sanitized URL", got)
	}
}

func TestEnrichRequestSanitizesURL(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		EnrichRequest(r, Update{URL: "https://user:pass@example.com/callback?code=secret#frag"})
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/tabs/tab-1/text", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].URL; got != "https://example.com/callback" {
		t.Fatalf("event.URL = %q, want sanitized URL", got)
	}
}

func TestMiddlewareUsesSourceHeaderOverride(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(HeaderPTSource, "dashboard")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].Source; got != "dashboard" {
		t.Fatalf("event.Source = %q, want dashboard", got)
	}
}

func TestMiddlewareUsesDashboardSourceForCookieAuth(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.AddCookie(&http.Cookie{
		Name:  authn.CookieName,
		Value: "session-token",
	})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].Source; got != "dashboard" {
		t.Fatalf("event.Source = %q, want dashboard", got)
	}
}

func TestMiddlewarePrefersExplicitSourceHeaderOverCookieAuth(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set(HeaderPTSource, "mcp")
	req.AddCookie(&http.Cookie{
		Name:  authn.CookieName,
		Value: "session-token",
	})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].Source; got != "mcp" {
		t.Fatalf("event.Source = %q, want mcp", got)
	}
}

func TestMiddlewareUsesAgentIDHeader(t *testing.T) {
	rec := &captureRecorder{}
	handler := Middleware(rec, "server", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(HeaderAgentID, "agent-main")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if len(rec.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(rec.events))
	}
	if got := rec.events[0].AgentID; got != "agent-main" {
		t.Fatalf("event.AgentID = %q, want agent-main", got)
	}
}

func TestPropagateHeadersUsesAgentIDHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	state := &requestState{event: Event{
		AgentID:   "agent-main",
		RequestID: "req-1",
	}}

	proxyReq := httptest.NewRequest(http.MethodGet, "http://example.test/health", nil)
	PropagateHeaders(context.WithValue(req.Context(), requestStateKey{}, state), proxyReq)

	if got := proxyReq.Header.Get(HeaderAgentID); got != "agent-main" {
		t.Fatalf("X-Agent-Id = %q, want agent-main", got)
	}
	if got := proxyReq.Header.Get("X-PinchTab-Agent-Id"); got != "" {
		t.Fatalf("X-PinchTab-Agent-Id = %q, want empty", got)
	}
}

// The enrichment peek is a budget for this package's own parse, never a limit on
// the request. Substituting the peeked bytes for r.Body truncated any action or
// navigate payload over the budget — the routes agents spend their traffic on —
// and the handler then reported the client's own JSON as invalid.
func TestAnOversizeRouteBodyReachesTheHandlerIntact(t *testing.T) {
	filler := strings.Repeat("x", activityPeekBytes*2)
	payload := `{"kind":"fill","ref":"e5","text":"` + filler + `"}`

	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(payload))
	EnrichRouteActivity(req)

	seen, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("the restored body could not be read: %v", err)
	}
	if string(seen) != payload {
		t.Fatalf("the handler saw %d of %d bytes; the middleware must peek without consuming", len(seen), len(payload))
	}
	var decoded map[string]any
	if err := json.Unmarshal(seen, &decoded); err != nil {
		t.Errorf("the body the handler received no longer parses (%v); PinchTab would report the client's payload as invalid", err)
	}
}

// A body within the budget still enriches, so the fix above did not turn the
// peek off.
func TestASmallRouteBodyStillEnrichesTheActivityRow(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(`{"kind":"click","ref":"e5"}`))
	state := &requestState{}
	req = req.WithContext(context.WithValue(req.Context(), requestStateKey{}, state))

	EnrichRouteActivity(req)

	if state.event.Action != "click" || state.event.Ref != "e5" {
		t.Errorf("action=%q ref=%q, want the peeked values", state.event.Action, state.event.Ref)
	}
	seen, _ := io.ReadAll(req.Body)
	if string(seen) != `{"kind":"click","ref":"e5"}` {
		t.Errorf("the enriched request lost its body: %q", string(seen))
	}
}
