package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// routeMockBridge implements just the slice of BridgeAPI used by the route
// handlers. Rule storage is in-memory per tab; CDP is never touched.
type routeMockBridge struct {
	mockBridge
	rules map[string][]bridge.RouteRule
}

func newRouteMockBridge() *routeMockBridge {
	return &routeMockBridge{rules: map[string][]bridge.RouteRule{}}
}

func (m *routeMockBridge) AddRouteRule(tabID string, rule bridge.RouteRule) error {
	for i, r := range m.rules[tabID] {
		if r.Pattern == rule.Pattern {
			m.rules[tabID][i] = rule
			return nil
		}
	}
	m.rules[tabID] = append(m.rules[tabID], rule)
	return nil
}

func (m *routeMockBridge) RemoveRouteRule(tabID, pattern string) (int, error) {
	if pattern == "" {
		n := len(m.rules[tabID])
		delete(m.rules, tabID)
		return n, nil
	}
	kept := m.rules[tabID][:0]
	removed := 0
	for _, r := range m.rules[tabID] {
		if r.Pattern == pattern {
			removed++
			continue
		}
		kept = append(kept, r)
	}
	m.rules[tabID] = kept
	return removed, nil
}

func (m *routeMockBridge) ListRouteRules(tabID string) ([]bridge.RouteRule, error) {
	return m.rules[tabID], nil
}

func newRouteHandler(b *routeMockBridge) *Handlers {
	return New(b, &config.RuntimeConfig{AllowNetworkIntercept: true}, nil, nil, nil)
}

func TestHandleTabNetworkRoute_AddAbort(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)

	body := `{"pattern":"*.png","action":"abort"}`
	req := httptest.NewRequest("POST", "/tabs/tab1/network/route", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkRoute(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK    bool               `json:"ok"`
		Rules []bridge.RouteRule `json:"rules"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.OK || len(resp.Rules) != 1 || resp.Rules[0].Action != bridge.RouteActionAbort {
		t.Errorf("unexpected response: %s", w.Body.String())
	}
}

func TestHandleTabNetworkRoute_AddFulfill_DefaultsContentType(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)

	body := `{"pattern":"api","action":"fulfill","body":"{\"k\":1}"}`
	req := httptest.NewRequest("POST", "/tabs/tab1/network/route", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkRoute(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := b.rules["tab1"][0]; got.Body != `{"k":1}` || got.Action != bridge.RouteActionFulfill {
		t.Errorf("rule not stored as expected: %+v", got)
	}
}

func TestHandleTabNetworkRoute_CapabilityDisabled(t *testing.T) {
	b := newRouteMockBridge()
	// Default RuntimeConfig has AllowNetworkIntercept=false.
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"pattern":"x","action":"continue"}`
	req := httptest.NewRequest("POST", "/tabs/tab1/network/route", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkRoute(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 when capability disabled, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "security.allowNetworkIntercept") {
		t.Errorf("expected setting key in error body, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "network_intercept_disabled") {
		t.Errorf("expected error code in body, got %s", w.Body.String())
	}
}

func TestNetworkInterceptSecurityStateIncludesEveryGatedDirectRoute(t *testing.T) {
	h := New(newRouteMockBridge(), &config.RuntimeConfig{}, nil, nil, nil)
	state := h.endpointSecurityStates()["networkIntercept"]
	want := []string{
		"GET /network/{requestId}",
		"GET /tabs/{id}/network/{requestId}",
		"POST /network/clear",
		"GET /network/route",
		"POST /network/route",
		"DELETE /network/route",
		"GET /tabs/{id}/network/route",
		"POST /tabs/{id}/network/route",
		"DELETE /tabs/{id}/network/route",
	}
	if len(state.Paths) != len(want) {
		t.Fatalf("security paths = %v, want %v", state.Paths, want)
	}
	for i := range want {
		if state.Paths[i] != want[i] {
			t.Fatalf("security path[%d] = %q, want %q", i, state.Paths[i], want[i])
		}
	}
}

func TestHandleTabNetworkUnroute_CapabilityDisabled(t *testing.T) {
	b := newRouteMockBridge()
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("DELETE", "/tabs/tab1/network/route", nil)
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkUnroute(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 when capability disabled, got %d", w.Code)
	}
}

func TestHandleTabNetworkRouteList_CapabilityDisabled(t *testing.T) {
	b := newRouteMockBridge()
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/tabs/tab1/network/route", nil)
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkRouteList(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 when capability disabled, got %d", w.Code)
	}
}

func TestHandleTabNetworkRoute_BodyExceedsCap(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)

	huge := make([]byte, bridge.MaxFulfillBodyBytes+1)
	for i := range huge {
		huge[i] = 'a'
	}
	payload, _ := json.Marshal(map[string]any{
		"pattern": "api",
		"action":  "fulfill",
		"body":    string(huge),
	})
	req := httptest.NewRequest("POST", "/tabs/tab1/network/route", bytes.NewReader(payload))
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkRoute(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for oversized body, got %d: %s", w.Code, w.Body.String())
	}
	if len(b.rules["tab1"]) != 0 {
		t.Errorf("oversized rule should not have been stored")
	}
}

func TestHandleTabNetworkRoute_StatusOutOfRange(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)

	for _, bad := range []int{99, 600, -1, 1000} {
		body := fmt.Sprintf(`{"pattern":"api","action":"fulfill","body":"{}","status":%d}`, bad)
		req := httptest.NewRequest("POST", "/tabs/tab1/network/route", bytes.NewReader([]byte(body)))
		req.SetPathValue("id", "tab1")
		w := httptest.NewRecorder()
		h.HandleTabNetworkRoute(w, req)
		if w.Code != 400 {
			t.Errorf("status=%d should reject (400), got %d: %s", bad, w.Code, w.Body.String())
		}
	}
}

func TestHandleTabNetworkUnroute_BadJSONReturns400(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)

	req := httptest.NewRequest("DELETE", "/tabs/tab1/network/route", bytes.NewReader([]byte(`{not json`)))
	req.SetPathValue("id", "tab1")
	req.ContentLength = 9
	w := httptest.NewRecorder()
	h.HandleTabNetworkUnroute(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for malformed body, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabNetworkRoute_MissingPattern(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)

	req := httptest.NewRequest("POST", "/tabs/tab1/network/route", bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkRoute(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabNetworkUnroute_All(t *testing.T) {
	b := newRouteMockBridge()
	b.rules["tab1"] = []bridge.RouteRule{
		{Pattern: "a", Action: bridge.RouteActionAbort},
		{Pattern: "b", Action: bridge.RouteActionAbort},
	}
	h := newRouteHandler(b)

	req := httptest.NewRequest("DELETE", "/tabs/tab1/network/route", nil)
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkUnroute(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Removed int `json:"removed"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Removed != 2 {
		t.Errorf("expected 2 removed, got %d", resp.Removed)
	}
	if len(b.rules["tab1"]) != 0 {
		t.Errorf("expected rules cleared, got %d", len(b.rules["tab1"]))
	}
}

func TestHandleTabNetworkUnroute_ByPatternQuery(t *testing.T) {
	b := newRouteMockBridge()
	b.rules["tab1"] = []bridge.RouteRule{
		{Pattern: "a", Action: bridge.RouteActionAbort},
		{Pattern: "b", Action: bridge.RouteActionAbort},
	}
	h := newRouteHandler(b)

	req := httptest.NewRequest("DELETE", "/tabs/tab1/network/route?pattern=a", nil)
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkUnroute(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(b.rules["tab1"]) != 1 || b.rules["tab1"][0].Pattern != "b" {
		t.Errorf("unexpected remaining rules: %+v", b.rules["tab1"])
	}
}

func TestHandleTabNetworkRouteList(t *testing.T) {
	b := newRouteMockBridge()
	b.rules["tab1"] = []bridge.RouteRule{
		{Pattern: "*.png", Action: bridge.RouteActionAbort},
	}
	h := newRouteHandler(b)

	req := httptest.NewRequest("GET", "/tabs/tab1/network/route", nil)
	req.SetPathValue("id", "tab1")
	w := httptest.NewRecorder()
	h.HandleTabNetworkRouteList(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Rules []bridge.RouteRule `json:"rules"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Rules) != 1 || resp.Rules[0].Pattern != "*.png" {
		t.Errorf("unexpected rules in response: %s", w.Body.String())
	}
}

func TestRegisterRoutes_Mounts(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	body := `{"pattern":"x","action":"continue"}`
	req := httptest.NewRequest("POST", "/tabs/tab1/network/route", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleNetworkRoute_BareForm(t *testing.T) {
	b := newRouteMockBridge()
	h := newRouteHandler(b)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	body := `{"pattern":"*.png","action":"abort"}`
	req := httptest.NewRequest("POST", "/network/route?tabId=tab1", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(b.rules["tab1"]) != 1 || b.rules["tab1"][0].Action != bridge.RouteActionAbort {
		t.Errorf("rule not stored on tab1 via bare form: %+v", b.rules["tab1"])
	}
}

func TestHandleNetworkUnroute_BareForm(t *testing.T) {
	b := newRouteMockBridge()
	b.rules["tab1"] = []bridge.RouteRule{
		{Pattern: "a", Action: bridge.RouteActionAbort},
		{Pattern: "b", Action: bridge.RouteActionAbort},
	}
	h := newRouteHandler(b)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("DELETE", "/network/route?tabId=tab1&pattern=a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(b.rules["tab1"]) != 1 || b.rules["tab1"][0].Pattern != "b" {
		t.Errorf("unexpected remaining rules: %+v", b.rules["tab1"])
	}
}

func TestHandleNetworkRouteList_BareForm(t *testing.T) {
	b := newRouteMockBridge()
	b.rules["tab1"] = []bridge.RouteRule{{Pattern: "*.css", Action: bridge.RouteActionAbort}}
	h := newRouteHandler(b)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("GET", "/network/route?tabId=tab1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
