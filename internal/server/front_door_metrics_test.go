package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/routes"
)

const frontDoorToken = "tok-front-door-1234567890"

// frontDoorFixture mounts the REAL front-door chain over a mux carrying the real
// local /metrics route. The rejections this card is about happen ABOVE the mux,
// in the auth middleware, so a test that exercised LoggingMiddleware on its own
// would pass while the product stayed blind — which is exactly what happened.
func frontDoorFixture(t *testing.T) http.Handler {
	t.Helper()
	handlers.ResetObservabilityForTests()
	t.Cleanup(handlers.ResetObservabilityForTests)

	cfg := &config.RuntimeConfig{Token: frontDoorToken, Bind: "127.0.0.1", Port: "9867"}
	mux := http.NewServeMux()
	registerFrontDoorMetrics(mux)
	mux.HandleFunc("GET /tabs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tabs":[]}`))
	})
	return FrontDoorHandler(config.NewLive(cfg), nil, nil, nil, mux)
}

func readFrontDoorMetrics(t *testing.T, handler http.Handler) map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+frontDoorToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode metrics: %v (body %s)", err, rec.Body.String())
	}
	return body
}

// The reported defect: N bad-token requests left requestsFailed at 0 forever,
// because /metrics was proxied to the instance and the front-door counters — the
// only ones that see an auth rejection — were unreachable by any client.
//
// The assertion is EXACT, not >=: an inequality passes on double counting, which
// is the specific risk when two layers of counters exist.
func TestFrontDoorMetricsCountRejectedRequestsExactly(t *testing.T) {
	handler := frontDoorFixture(t)

	const rejected = 5
	for i := 0; i < rejected; i++ {
		req := httptest.NewRequest("GET", "/tabs", nil)
		req.Header.Set("Authorization", "Bearer WRONG")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d = %d, want 401: %s", i, rec.Code, rec.Body.String())
		}
	}
	// An unrouted path is the other case that never reaches an instance.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", fmt.Sprintf("/zzz%d", i), nil)
		req.Header.Set("Authorization", "Bearer "+frontDoorToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("unrouted request %d = %d, want 404", i, rec.Code)
		}
	}

	body := readFrontDoorMetrics(t, handler)
	if body["layer"] != handlers.LayerFrontDoor {
		t.Fatalf("layer = %v, want %q — a number that does not say which process it describes is not actionable", body["layer"], handlers.LayerFrontDoor)
	}

	failures, ok := body["failures"].(map[string]any)
	if !ok {
		t.Fatalf("no failures block in front-door metrics: %v", body)
	}
	if got := failures["requestsFailed"].(float64); got != rejected+2 {
		t.Errorf("requestsFailed = %v, want exactly %d", got, rejected+2)
	}
	recent, ok := failures["recent"].([]any)
	if !ok {
		t.Fatalf("failures.recent missing: %v", failures)
	}
	if len(recent) != rejected+2 {
		t.Fatalf("failures.recent has %d events, want exactly %d", len(recent), rejected+2)
	}

	// Every event must carry the four fields an operator needs to act, plus the
	// layer that produced it.
	unauthorized := 0
	for i, raw := range recent {
		event, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("event %d is not an object: %v", i, raw)
		}
		for _, key := range []string{"method", "path", "status", "requestId", "layer"} {
			if event[key] == nil || event[key] == "" {
				t.Errorf("event %d missing %q: %v", i, key, event)
			}
		}
		if event["layer"] != handlers.LayerFrontDoor {
			t.Errorf("event %d layer = %v, want %q", i, event["layer"], handlers.LayerFrontDoor)
		}
		if event["status"].(float64) == http.StatusUnauthorized {
			unauthorized++
			if event["path"] != "/tabs" {
				t.Errorf("401 event %d path = %v, want /tabs", i, event["path"])
			}
			if event["method"] != "GET" {
				t.Errorf("401 event %d method = %v, want GET", i, event["method"])
			}
		}
	}
	if unauthorized != rejected {
		t.Errorf("recorded %d auth rejections, want %d", unauthorized, rejected)
	}
}

// crashes.recent must be present in server mode too, and with a stable shape: a
// key that only appears after the first crash cannot be relied on by a client
// watching for one.
func TestFrontDoorMetricsAlwaysCarryFailureAndCrashBlocks(t *testing.T) {
	body := readFrontDoorMetrics(t, frontDoorFixture(t))

	for _, key := range []string{"metrics", "failures", "crashes", "layer"} {
		if _, ok := body[key]; !ok {
			t.Errorf("front-door metrics missing %q on a clean server: %v", key, body)
		}
	}
	crashes, ok := body["crashes"].(map[string]any)
	if !ok {
		t.Fatalf("crashes block is not an object: %v", body["crashes"])
	}
	if _, ok := crashes["recent"]; !ok {
		t.Errorf("crashes.recent missing: %v", crashes)
	}
	failures := body["failures"].(map[string]any)
	if got := failures["requestsFailed"].(float64); got != 0 {
		t.Errorf("a clean front door reports requestsFailed = %v, want 0", got)
	}
}

// The front door can only answer /metrics itself if the proxy registration stops
// claiming it — registering both patterns panics. This pins the split rather than
// the spelling: every CapNone endpoint belongs to exactly one of the two sets.
func TestMetricsIsFrontDoorLocalAndNotProxied(t *testing.T) {
	local := routes.FrontDoorLocalRoutes()
	proxied := routes.ShorthandRoutes()

	if len(local) == 0 {
		t.Fatal("no front-door-local routes; the front door would proxy its own diagnostics again")
	}
	inLocal := map[string]bool{}
	for _, route := range local {
		inLocal[route] = true
	}
	for _, route := range proxied {
		if inLocal[route] {
			t.Errorf("%q is registered both locally and as a proxy route; mux registration would panic", route)
		}
	}
	if !inLocal["GET /metrics"] {
		t.Errorf("GET /metrics must be answered by the front door: local routes = %v", local)
	}

	// The tab-scoped variant is a browser measurement and must STILL proxy.
	for _, route := range routes.TabScopedRoutes() {
		if route == "GET /tabs/{id}/metrics" {
			return
		}
	}
	t.Error("GET /tabs/{id}/metrics is no longer proxied; per-tab metrics belong to the instance")
}
