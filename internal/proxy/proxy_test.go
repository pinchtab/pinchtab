package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"net/url"
	"strings"
	"time"
)

func fakeBridge(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"proxied": true,
			"path":    r.URL.Path,
			"query":   r.URL.RawQuery,
		})
	}))
}

func TestHTTP_ForwardsRequest(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, srv.URL+"/snapshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["path"] != "/snapshot" {
		t.Errorf("expected path /snapshot, got %v", resp["path"])
	}
}

func TestHTTP_ForwardsQueryParams(t *testing.T) {
	srv := fakeBridge(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/screenshot?raw=true", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, srv.URL+"/screenshot")

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["query"] != "raw=true" {
		t.Errorf("expected query raw=true, got %v", resp["query"])
	}
}

func TestHTTP_UnreachableReturns502(t *testing.T) {
	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, "http://localhost:1/snapshot")

	if rec.Code != 502 {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}

func TestHTTP_CopiesResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(201)
	}))
	defer srv.Close()

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	HTTP(rec, req, srv.URL+"/test")

	if rec.Code != 201 {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom") != "test-value" {
		t.Errorf("expected X-Custom header, got %q", rec.Header().Get("X-Custom"))
	}
}

func TestHTTP_StripsSensitiveRequestHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization":   r.Header.Get("Authorization"),
			"cookie":          r.Header.Get("Cookie"),
			"xForwardedFor":   r.Header.Get("X-Forwarded-For"),
			"xForwardedHost":  r.Header.Get("X-Forwarded-Host"),
			"xForwardedProto": r.Header.Get("X-Forwarded-Proto"),
			"forwarded":       r.Header.Get("Forwarded"),
			"xRealIP":         r.Header.Get("X-Real-Ip"),
			"xRequestID":      r.Header.Get("X-Request-Id"),
		})
	}))
	defer srv.Close()

	req := httptest.NewRequest("GET", "/snapshot", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Set("Cookie", "pinchtab_auth_token=session-secret")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("X-Forwarded-Host", "app.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Forwarded", `for=203.0.113.10;host=app.example;proto=https`)
	req.Header.Set("X-Real-Ip", "203.0.113.10")
	req.Header.Set("X-Request-Id", "req-123")

	rec := httptest.NewRecorder()
	HTTP(rec, req, srv.URL+"/snapshot")

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["authorization"] != "Bearer user-token" {
		t.Fatalf("authorization = %v, want preserved bearer token", resp["authorization"])
	}
	for _, field := range []string{"cookie", "xForwardedFor", "xForwardedHost", "xForwardedProto", "forwarded", "xRealIP"} {
		if got := resp[field]; got != "" {
			t.Fatalf("%s should have been stripped, got %v", field, got)
		}
	}

	// X-Request-Id is the deliberate exception to this strip list, and it is asserted in
	// the same test as the members that stay stripped so the two cannot drift apart.
	// Forwarding it is what makes one proxied request findable in the instance log as well
	// as the outer one; without it the instance mints an id of its own that appears in no
	// log a caller could be pointed at. The value is client-influenced here on purpose —
	// RequestIDMiddleware honours an inbound id by design and the outer server already
	// logs it, so the instance learns nothing the outer log does not already contain.
	if got := resp["xRequestID"]; got != "req-123" {
		t.Fatalf("xRequestID = %v, want the outer id req-123 forwarded so both logs carry it", got)
	}
}

func TestHTTP_UsesSharedClient(t *testing.T) {
	if DefaultClient == nil {
		t.Fatal("DefaultClient should not be nil")
	}
	if DefaultClient.Timeout != httpx.MaxNavigationHTTPDuration {
		t.Errorf("timeout = %s, want %s", DefaultClient.Timeout, httpx.MaxNavigationHTTPDuration)
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   bool
	}{
		{"no upgrade", http.Header{}, false},
		{"websocket", http.Header{"Upgrade": {"websocket"}}, true},
		{"WebSocket", http.Header{"Upgrade": {"WebSocket"}}, true},
		{"other", http.Header{"Upgrade": {"h2c"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header = tt.header
			if got := isWebSocketUpgrade(r); got != tt.want {
				t.Errorf("isWebSocketUpgrade() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTP_DoesNotDoubleTheOuterChainsResponseHeaders(t *testing.T) {
	owned := httpx.OuterChainResponseHeaders()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range owned {
			w.Header().Set(name, "instance-"+name)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	req := httptest.NewRequest("GET", "/tabs", nil)
	rec := httptest.NewRecorder()
	for _, name := range owned {
		rec.Header().Set(name, "outer-"+name)
	}

	HTTP(rec, req, srv.URL+"/tabs")

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	for _, name := range owned {
		got := rec.Header().Values(name)
		if len(got) != 1 {
			t.Errorf("%s = %v, want exactly one value — the second is minted by the instance and appears in no log the outer process writes", name, got)
			continue
		}
		if got[0] != "outer-"+name {
			t.Errorf("%s = %q, want the outer chain's value, which is the one it logged", name, got[0])
		}
	}
}

// wedgedInstance answers /health instantly and holds /tabs and /navigate for
// `hold`: a stub that blocked everything would exercise connection failure,
// which already worked, not the read budget this pins.
func wedgedInstance(t *testing.T, hold time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tabs", "/navigate":
			select {
			case <-time.After(hold):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"path":"` + r.URL.Path + `"}`))
	}))
}

func shrinkBudgets(t *testing.T, read, long time.Duration) {
	t.Helper()
	prevRead, prevLong := httpx.ReadHTTPDuration, httpx.LongOperationHTTPBudget
	httpx.ReadHTTPDuration, httpx.LongOperationHTTPBudget = read, long
	t.Cleanup(func() { httpx.ReadHTTPDuration, httpx.LongOperationHTTPBudget = prevRead, prevLong })
}

// A wedged instance costs a read its own short budget, names the instance and
// the budget, and a navigate through the same client still gets the long ceiling.
func TestForwardBoundsAReadByItsOwnBudgetAndLeavesNavigationItsCeiling(t *testing.T) {
	shrinkBudgets(t, 100*time.Millisecond, 2*time.Second)
	srv := wedgedInstance(t, 400*time.Millisecond)
	defer srv.Close()
	target := func(path string) *url.URL {
		u, _ := url.Parse(srv.URL + path)
		return u
	}
	rewrite := func(req *http.Request) { req.Header.Set(activity.HeaderPTInstance, "inst_wedged") }

	started := time.Now()
	rec := httptest.NewRecorder()
	Forward(rec, httptest.NewRequest("GET", "/tabs", nil), target("/tabs"), Options{RewriteRequest: rewrite})
	if rec.Code != 502 {
		t.Fatalf("GET /tabs on a wedged instance = %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if waited := time.Since(started); waited > time.Second {
		t.Fatalf("the read waited %v; the read budget did not bound it", waited)
	}
	for _, want := range []string{"instance inst_wedged", "did not answer within its 100ms budget"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("failure does not say %q: %s", want, rec.Body.String())
		}
	}

	rec = httptest.NewRecorder()
	Forward(rec, httptest.NewRequest("GET", "/health", nil), target("/health"), Options{})
	if rec.Code != 200 {
		t.Fatalf("GET /health = %d; the budget must not touch an instance that answers", rec.Code)
	}

	rec = httptest.NewRecorder()
	Forward(rec, httptest.NewRequest("POST", "/navigate", strings.NewReader(`{"url":"https://example.com"}`)), target("/navigate"), Options{})
	if rec.Code != 200 {
		t.Fatalf("POST /navigate = %d: %s; navigation lost its long ceiling", rec.Code, rec.Body.String())
	}
}
