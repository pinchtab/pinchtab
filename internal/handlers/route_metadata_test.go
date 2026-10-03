package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/config"
)

func routeTestConfig() *config.RuntimeConfig {
	return &config.RuntimeConfig{
		ActionTimeout:     time.Second,
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak, "ghost-chrome"},
	}
}

func assertSingleAttempt(t *testing.T, route *browserops.RouteMetadata, browser string) {
	t.Helper()
	if route == nil {
		t.Fatal("expected route metadata, got nil")
	}
	if len(route.Attempts) != 1 {
		t.Fatalf("len(Attempts) = %d, want 1: %+v", len(route.Attempts), route.Attempts)
	}
	if route.Attempts[0].Browser != browser || !route.Attempts[0].Accepted {
		t.Fatalf("Attempts[0] = %+v, want accepted %q", route.Attempts[0], browser)
	}
	if route.Escalated {
		t.Errorf("Escalated = true, want false on the plain handle path")
	}
	if route.UsedBrowser != browser {
		t.Errorf("UsedBrowser = %q, want %q", route.UsedBrowser, browser)
	}
}

func assertGhostChromeDowngrade(t *testing.T, route *browserops.RouteMetadata) {
	t.Helper()
	if route == nil {
		t.Fatal("expected route metadata, got nil")
	}
	if len(route.Attempts) != 2 {
		t.Fatalf("len(Attempts) = %d, want 2 (ghost-chrome then chrome): %+v", len(route.Attempts), route.Attempts)
	}
	skipped, ran := route.Attempts[0], route.Attempts[1]
	if skipped.Browser != "ghost-chrome" || skipped.Accepted {
		t.Errorf("Attempts[0] = %+v, want ghost-chrome rejected", skipped)
	}
	if skipped.Reason == "" {
		t.Error("Attempts[0].Reason is empty, want the skipping browser's own reason")
	}
	if ran.Browser != config.BrowserChrome || !ran.Accepted {
		t.Errorf("Attempts[1] = %+v, want chrome accepted", ran)
	}
	for _, attempt := range route.Attempts {
		if attempt.Browser == config.BrowserChrome && !attempt.Accepted {
			t.Errorf("chrome recorded as rejected (%+v); chrome is the browser that ran", attempt)
		}
	}
	if !route.Escalated {
		t.Error("Escalated = false, want true on a capability downgrade")
	}
	if route.UsedBrowser != config.BrowserChrome {
		t.Errorf("UsedBrowser = %q, want %q", route.UsedBrowser, config.BrowserChrome)
	}
	if route.RequestedBrowser != "ghost-chrome" {
		t.Errorf("RequestedBrowser = %q, want %q", route.RequestedBrowser, "ghost-chrome")
	}
}

func readRoute(t *testing.T, h *Handlers, browser, recordName, shape string) *browserops.RouteMetadata {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/"+recordName+"?browser="+browser, nil)
	_, route, ok := h.resolveReadRouting(w, r, "", recordName, shape)
	if !ok {
		t.Fatalf("resolveReadRouting(%s) failed: %d %s", recordName, w.Code, w.Body.String())
	}
	return route
}

func TestResolveReadRouting_SingleBrowserRecordsOneAttempt(t *testing.T) {
	reads := []struct {
		name  string
		shape string
	}{
		{"text", browsers.ShapeRenderedRead},
		{"snapshot", browsers.ShapeStaticSnapshot},
		{"capture", browsers.ShapeVisual},
	}

	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			h := New(&mockBridge{}, routeTestConfig(), nil, nil, nil)
			assertSingleAttempt(t, readRoute(t, h, config.BrowserCloak, read.name, read.shape), config.BrowserCloak)
		})
	}
}

func TestResolveReadRouting_GhostChromeDowngradeRecordsBothBrowsers(t *testing.T) {
	h := New(&mockBridge{}, routeTestConfig(), nil, nil, nil)
	assertGhostChromeDowngrade(t, readRoute(t, h, "ghost-chrome", "text", browsers.ShapeRenderedRead))
}

type capturingRecorder struct{ events []activity.Event }

func (c *capturingRecorder) Enabled() bool { return true }

func (c *capturingRecorder) Record(evt activity.Event) error {
	c.events = append(c.events, evt)
	return nil
}

func (c *capturingRecorder) Query(activity.Filter) ([]activity.Event, error) { return c.events, nil }

type routedReply struct {
	route    *browserops.RouteMetadata
	hasRoute bool
	recorded *browserops.RouteMetadata
}

func serveRouted(t *testing.T, handler http.HandlerFunc, req *http.Request) routedReply {
	t.Helper()
	rec := &capturingRecorder{}
	w := httptest.NewRecorder()
	activity.Middleware(rec, "client", handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rec.events) != 1 {
		t.Fatalf("recorded %d activity events, want 1", len(rec.events))
	}
	reply := routedReply{recorded: rec.events[0].Route}
	raw, ok := resp["route"]
	if !ok {
		return reply
	}
	reply.hasRoute = true
	if err := json.Unmarshal(raw, &reply.route); err != nil {
		t.Fatalf("decode route %s: %v", raw, err)
	}
	return reply
}

func assertTrivialRouteOnlyRecorded(t *testing.T, reply routedReply, browser string) {
	t.Helper()
	if reply.hasRoute {
		t.Fatalf("reply carries a route for an unrouted request: %+v", reply.route)
	}
	assertSingleAttempt(t, reply.recorded, browser)
}

func assertDowngradeRepliedAndRecorded(t *testing.T, reply routedReply) {
	t.Helper()
	assertGhostChromeDowngrade(t, reply.route)
	assertGhostChromeDowngrade(t, reply.recorded)
}

func actionReply(t *testing.T, browser string) routedReply {
	t.Helper()
	h := New(&autoSwitchActionBridge{}, routeTestConfig(), nil, nil, nil)
	body := []byte(`{"kind":"click","browser":"` + browser + `"}`)
	return serveRouted(t, h.HandleAction, httptest.NewRequest("POST", "/action", bytes.NewReader(body)))
}

func TestHandleAction_SingleBrowserRecordsOneAttempt(t *testing.T) {
	assertTrivialRouteOnlyRecorded(t, actionReply(t, config.BrowserCloak), config.BrowserCloak)
}

func TestHandleAction_GhostChromeDowngradeRecordsBothBrowsers(t *testing.T) {
	assertDowngradeRepliedAndRecorded(t, actionReply(t, "ghost-chrome"))
}

func multiStepReply(t *testing.T, path, body string) routedReply {
	t.Helper()
	cfg := routeTestConfig()
	cfg.AllowMacro = true
	h := New(&autoSwitchActionBridge{}, cfg, nil, nil, nil)
	req := httptest.NewRequest("POST", path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if path == "/macro" {
		return serveRouted(t, h.HandleMacro, req)
	}
	return serveRouted(t, h.HandleActions, req)
}

func TestHandleActions_SingleBrowserRecordsOneAttempt(t *testing.T) {
	assertTrivialRouteOnlyRecorded(t, multiStepReply(t, "/actions", `{"actions":[{"kind":"click","browser":"cloak"}]}`), config.BrowserCloak)
}

func TestHandleActions_GhostChromeDowngradeRecordsBothBrowsers(t *testing.T) {
	assertDowngradeRepliedAndRecorded(t, multiStepReply(t, "/actions", `{"actions":[{"kind":"click","browser":"ghost-chrome"}]}`))
}

func TestHandleMacro_SingleBrowserRecordsOneAttempt(t *testing.T) {
	assertTrivialRouteOnlyRecorded(t, multiStepReply(t, "/macro", `{"steps":[{"kind":"click","browser":"cloak"}]}`), config.BrowserCloak)
}

func TestHandleMacro_GhostChromeDowngradeRecordsBothBrowsers(t *testing.T) {
	assertDowngradeRepliedAndRecorded(t, multiStepReply(t, "/macro", `{"steps":[{"kind":"click","browser":"ghost-chrome"}]}`))
}

func navigateReply(t *testing.T, browser string) routedReply {
	t.Helper()
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	m := &mockBridge{navigateResult: &bridge.NavigateResult{TabID: "tab_abc12345", URL: "https://example.com/"}}
	h := New(m, routeTestConfig(), nil, nil, nil)
	body := []byte(`{"url":"https://example.com","browser":"` + browser + `"}`)
	return serveRouted(t, h.HandleNavigate, httptest.NewRequest("POST", "/navigate", bytes.NewReader(body)))
}

func TestHandleNavigate_SingleBrowserRecordsOneAttempt(t *testing.T) {
	assertTrivialRouteOnlyRecorded(t, navigateReply(t, config.BrowserCloak), config.BrowserCloak)
}

func TestHandleNavigate_GhostChromeDowngradeRecordsBothBrowsers(t *testing.T) {
	assertDowngradeRepliedAndRecorded(t, navigateReply(t, "ghost-chrome"))
}

func TestRouteMetadataTrivial(t *testing.T) {
	oneAttempt := []browserops.RouteAttempt{{Browser: "cloak", Accepted: true}}
	for name, tc := range map[string]struct {
		route *browserops.RouteMetadata
		want  bool
	}{
		"nil":               {nil, true},
		"unrouted":          {&browserops.RouteMetadata{RequestedBrowser: "cloak", UsedBrowser: "cloak", Attempts: oneAttempt}, true},
		"downgraded":        {&browserops.RouteMetadata{RequestedBrowser: "ghost-chrome", UsedBrowser: "cloak", Attempts: oneAttempt}, false},
		"escalated":         {&browserops.RouteMetadata{RequestedBrowser: "cloak", UsedBrowser: "cloak", Escalated: true, Attempts: oneAttempt}, false},
		"reason":            {&browserops.RouteMetadata{RequestedBrowser: "cloak", UsedBrowser: "cloak", Reason: "x", Attempts: oneAttempt}, false},
		"fallback attempts": {&browserops.RouteMetadata{RequestedBrowser: "cloak", UsedBrowser: "cloak", FallbackAttempts: 1, Attempts: oneAttempt}, false},
		"two attempts":      {&browserops.RouteMetadata{RequestedBrowser: "cloak", UsedBrowser: "cloak", Attempts: []browserops.RouteAttempt{{}, {}}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.route.Trivial(); got != tc.want {
				t.Fatalf("Trivial() = %v, want %v", got, tc.want)
			}
		})
	}
}
