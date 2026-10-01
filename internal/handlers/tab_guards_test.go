package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/pinchtab/pinchtab/internal/state"
)

// TestEveryBindingDeclaresItsGuardSet is the reason routeBinding carries guards
// at all: an endpoint's tab policy has to be read off one catalog line, so the
// no-guard case is written as guardNone rather than left to the zero value.
func TestEveryBindingDeclaresItsGuardSet(t *testing.T) {
	seen := 0
	for _, b := range (*Handlers)(nil).bridgeBindings() {
		seen++
		if b.guards == 0 {
			t.Errorf("%s declares no guard set; write guards: guardNone so the default is declared rather than inherited by omission", b.pattern)
			continue
		}
		if b.guards&guardNone != 0 && b.guards != guardNone {
			t.Errorf("%s combines guardNone with a real guard, so its line does not say what it enforces", b.pattern)
		}
	}
	if seen == 0 {
		t.Fatal("bridgeBindings() is empty, so this guard proves nothing")
	}
}

// guardProbeBridge answers the two state lookups the guard chain consults —
// handoff status and the tab's current URL — and nothing else, so a probe
// observes the declared guard rather than whatever the handler does next.
type guardProbeBridge struct {
	mockBridge
	paused     bool
	currentURL string
	dialogs    *bridgetabs.DialogManager
	lockOwner  string
}

func (b *guardProbeBridge) TabLockInfo(string) *bridge.LockInfo {
	if b.lockOwner == "" {
		return nil
	}
	return &bridge.LockInfo{Owner: b.lockOwner}
}

// probePausedAt is fixed so two refusals from different endpoints differ only
// where they are genuinely allowed to; a live clock would put a different
// pausedAt in each body and make the envelope comparison below untestable.
var probePausedAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

func pendingDialogManager() *bridgetabs.DialogManager {
	dm := bridgetabs.NewDialogManager()
	dm.SetPending("tab1", &bridge.DialogState{Type: "confirm", Message: "probe"})
	return dm
}

func (b *guardProbeBridge) SetTabHandoff(string, string, time.Duration) error { return nil }

func (b *guardProbeBridge) ResumeTabHandoff(string) error { return nil }

func (b *guardProbeBridge) TabHandoffState(string) (bridge.TabHandoffState, bool) {
	if !b.paused {
		return bridge.TabHandoffState{Status: "active"}, true
	}
	return bridge.TabHandoffState{
		Status:        "paused_handoff",
		Reason:        "manual_handoff",
		PausedAt:      probePausedAt,
		LastUpdatedAt: probePausedAt,
	}, true
}

func (b *guardProbeBridge) CurrentURL(context.Context) (string, error) { return b.currentURL, nil }

// GetDialogManager reports no dialog manager unless the probe seeds one, so
// guardDialogBlocked resolves to "no dialog" without CDP work: it runs first in
// the chain, and a probe that fails inside it would never reach the guard under
// test.
func (b *guardProbeBridge) GetDialogManager() *bridgetabs.DialogManager { return b.dialogs }

// guardProbe carries what a route needs to reach its guards. Guards run after
// each handler's own validation, so a route that rejects {} never gets far
// enough to refuse and its declaration would go unverified — the conformance
// test fails naming the route until an entry lands here. wildcard fills path
// wildcards other than {id}.
type guardProbe struct {
	body     string
	query    string
	wildcard string
}

var guardProbes = map[string]guardProbe{
	"POST /navigate":              {body: `{"url":"http://127.0.0.1:9/"}`},
	"POST /tab":                   {body: `{"action":"new","url":"http://127.0.0.1:9/"}`},
	"POST /action":                {body: `{"kind":"click","selector":"#probe"}`},
	"POST /actions":               {body: `{"actions":[{"kind":"click","selector":"#probe"}]}`},
	"POST /macro":                 {body: `{"steps":[{"kind":"click","selector":"#probe"}]}`},
	"POST /dialog":                {body: `{"action":"accept"}`},
	"POST /wait":                  {body: `{"selector":"#probe"}`},
	"POST /find":                  {body: `{"query":"probe"}`},
	"POST /extract":               {body: `{"schema":{"type":"object","properties":{"probe":{"type":"string"}}}}`},
	"POST /evaluate":              {body: `{"expression":"1"}`},
	"POST /upload":                {body: `{"selector":"#probe","paths":["probe.txt"]}`},
	"POST /solve/{name}":          {wildcard: "cloudflare"},
	"POST /cookies":               {body: `{"cookies":[{"name":"probe","value":"1","domain":"127.0.0.1"}]}`},
	"POST /emulation/viewport":    {body: `{"width":800,"height":600}`},
	"POST /emulation/headers":     {body: `{"headers":{"X-Probe":"1"}}`},
	"POST /emulation/credentials": {body: `{"username":"probe","password":"probe"}`},
	"POST /emulation/media":       {body: `{"feature":"prefers-color-scheme","value":"dark"}`},
	"POST /storage":               {body: `{"type":"local","key":"probe","value":"1"}`},
	"POST /state/load":            {body: `{"name":"probe"}`},
	"GET /value":                  {query: "selector=%23probe"},
	"GET /attr":                   {query: "selector=%23probe&name=id"},
	"GET /count":                  {query: "selector=%23probe"},
	"GET /box":                    {query: "selector=%23probe"},
	"GET /visible":                {query: "selector=%23probe"},
	"GET /enabled":                {query: "selector=%23probe"},
	"GET /checked":                {query: "selector=%23probe"},
	"GET /download":               {query: "url=http%3A%2F%2Fexample.com%2Ff.bin"},
	"GET /network/export/stream":  {query: "path=probe.har"},
}

// allCapabilities opens every capability gate: a 403 capability refusal answers
// before the guard chain, which would make every gated route look unguarded.
func allCapabilities(t *testing.T, tmpDir string) *config.RuntimeConfig {
	t.Helper()
	sandbox := filepath.Join(tmpDir, uploadSandboxDirName)
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		t.Fatalf("seed upload sandbox: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sandbox, "probe.txt"), []byte("probe"), 0o644); err != nil {
		t.Fatalf("seed upload probe file: %v", err)
	}
	sessions := state.SessionsDir(tmpDir)
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatalf("seed state sessions dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessions, "probe.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed state probe file: %v", err)
	}
	return &config.RuntimeConfig{
		ActionTimeout:         time.Second,
		StateDir:              tmpDir,
		AllowEvaluate:         true,
		AllowMacro:            true,
		AllowScreencast:       true,
		AllowDownload:         true,
		AllowCookies:          true,
		AllowNetworkIntercept: true,
		AllowUpload:           true,
		AllowStateExport:      true,
		AllowMemory:           true,

		DownloadAllowedDomains: []string{"example.com"},
	}
}

// probeRoute drives one catalog endpoint through the real mux and reports the
// refusal code it answered with. answered=false means the handler panicked on
// the probe bridge, which is past the guard chain and so proves nothing either
// way.
func probeRoute(t *testing.T, b bridge.BridgeAPI, cfg *config.RuntimeConfig, ep routes.Endpoint) (code string, status int, answered bool) {
	t.Helper()
	w, answered := recordProbeRoute(t, b, cfg, ep)
	var resp struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Code, w.Code, answered
}

func recordProbeRoute(t *testing.T, b bridge.BridgeAPI, cfg *config.RuntimeConfig, ep routes.Endpoint) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	return recordProbeRouteAs(t, b, cfg, ep, "")
}

func recordProbeRouteAs(t *testing.T, b bridge.BridgeAPI, cfg *config.RuntimeConfig, ep routes.Endpoint, owner string) (*httptest.ResponseRecorder, bool) {
	t.Helper()

	h := New(b, cfg, nil, nil, nil)
	mux := http.NewServeMux()
	h.registerBridgeRoutes(mux)

	route := ep.Route()
	if ep.TabScoped {
		route = ep.TabRoute()
	}
	probe := guardProbes[ep.Route()]
	method, path, _ := strings.Cut(route, " ")
	path = strings.ReplaceAll(path, "{id}", "tab1")
	if probe.wildcard != "" {
		path = strings.ReplaceAll(path, "{name}", probe.wildcard)
	}
	path = concreteRoutePath(path)
	if probe.query != "" {
		path += "?" + probe.query
	}

	w := httptest.NewRecorder()
	answered := true
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recover() != nil {
				answered = false
			}
		}()
		body := probe.body
		if body == "" {
			body = "{}"
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if owner != "" {
			req.Header.Set("X-Owner", owner)
		}
		mux.ServeHTTP(w, req)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not answer within the deadline", route)
	}

	return w, answered
}

// guardedBindings pairs every catalog endpoint with the binding that declares
// its guard set, so the conformance checks below walk the catalog rather than a
// hand-kept list that could quietly stop covering a route.
func guardedBindings(t *testing.T) []struct {
	ep routes.Endpoint
	b  routeBinding
} {
	t.Helper()
	bind := map[string]routeBinding{}
	for _, b := range (*Handlers)(nil).bridgeBindings() {
		bind[b.pattern] = b
	}
	var out []struct {
		ep routes.Endpoint
		b  routeBinding
	}
	for _, ep := range routes.Core() {
		b, ok := bind[ep.Route()]
		if !ok {
			t.Fatalf("catalog route %s has no binding", ep.Route())
		}
		out = append(out, struct {
			ep routes.Endpoint
			b  routeBinding
		}{ep, b})
	}
	if len(out) == 0 {
		t.Fatal("the catalog is empty, so these conformance checks prove nothing")
	}
	return out
}

// TestDeclaredHandoffGuardMatchesTheRefusal is what stops the guards field being
// decorative: every route that declares guardHandoffPause must actually refuse a
// paused tab, and no route that omits it may. IDPI is off here so the domain
// guard cannot answer first and mask the one under test.
func TestDeclaredHandoffGuardMatchesTheRefusal(t *testing.T) {
	checked, refusals := 0, 0
	for _, row := range guardedBindings(t) {
		t.Run(row.ep.Route(), func(t *testing.T) {
			b := &guardProbeBridge{paused: true, currentURL: "https://allowed.example/"}
			code, status, answered := probeRoute(t, b, allCapabilities(t, t.TempDir()), row.ep)
			refused := code == handoffPausedCode
			declared := row.b.guards&guardHandoffPause != 0

			switch {
			case declared && !refused:
				t.Errorf("declares guardHandoffPause but a paused tab answered %d %q; either the declaration is wrong or the route needs a guardProbes entry that reaches its guards", status, code)
			case !declared && refused && answered:
				t.Errorf("refuses a paused tab without declaring guardHandoffPause, so its catalog line understates what it enforces")
			}
			checked++
			if refused {
				refusals++
			}
		})
	}
	if checked == 0 || refusals == 0 {
		t.Fatalf("checked %d routes and saw %d handoff refusals; with none observed this test cannot tell a guard from its absence", checked, refusals)
	}
}

// TestDeclaredDomainGuardMatchesTheRefusal is the same conformance check for the
// IDPI current-tab domain policy, with the tab parked on a domain the allowlist
// does not carry and handoff left active so it cannot answer instead.
func TestDeclaredDomainGuardMatchesTheRefusal(t *testing.T) {
	cfgFor := func(t *testing.T, tmpDir string) *config.RuntimeConfig {
		cfg := allCapabilities(t, tmpDir)
		cfg.IDPI = config.IDPIConfig{Enabled: true, StrictMode: true}
		cfg.AllowedDomains = []string{"allowed.example", "127.0.0.1"}
		return cfg
	}

	checked, refusals := 0, 0
	for _, row := range guardedBindings(t) {
		t.Run(row.ep.Route(), func(t *testing.T) {
			b := &guardProbeBridge{currentURL: "https://blocked.example/page"}
			code, status, answered := probeRoute(t, b, cfgFor(t, t.TempDir()), row.ep)
			refused := code == idpiDomainBlockedCode
			declared := row.b.guards&guardDomainPolicy != 0

			switch {
			case declared && !refused:
				t.Errorf("declares guardDomainPolicy but an off-allowlist tab answered %d %q; either the declaration is wrong or the route needs a guardProbes entry that reaches its guards", status, code)
			case !declared && refused && answered:
				t.Errorf("blocks an off-allowlist tab without declaring guardDomainPolicy, so its catalog line understates what it enforces")
			}
			checked++
			if refused {
				refusals++
			}
		})
	}
	if checked == 0 || refusals == 0 {
		t.Fatalf("checked %d routes and saw %d domain refusals; with none observed this test cannot tell a guard from its absence", checked, refusals)
	}
}

// TestDeclaredDialogGuardMatchesTheRefusal closes the third flag in the guard
// vocabulary: a route that declares guardDialogBlocked must refuse while a
// dialog is pending on the tab, and no route that omits it may.
func TestDeclaredDialogGuardMatchesTheRefusal(t *testing.T) {
	checked, refusals := 0, 0
	for _, row := range guardedBindings(t) {
		t.Run(row.ep.Route(), func(t *testing.T) {
			b := &guardProbeBridge{currentURL: "https://allowed.example/", dialogs: pendingDialogManager()}
			code, status, answered := probeRoute(t, b, allCapabilities(t, t.TempDir()), row.ep)
			refused := code == dialogBlockedCode
			declared := row.b.guards&guardDialogBlocked != 0

			switch {
			case declared && !refused:
				t.Errorf("declares guardDialogBlocked but a dialog-blocked tab answered %d %q; either the declaration is wrong or the route needs a guardProbes entry that reaches its guards", status, code)
			case !declared && refused && answered:
				t.Errorf("refuses a dialog-blocked tab without declaring guardDialogBlocked, so its catalog line understates what it enforces")
			}
			checked++
			if refused {
				refusals++
			}
		})
	}
	if checked == 0 || refusals == 0 {
		t.Fatalf("checked %d routes and saw %d dialog refusals; with none observed this test cannot tell a guard from its absence", checked, refusals)
	}
}

// newlyGuardedMutators are the endpoints the mutating-tab ruling moved onto the
// handoff-pause guard. Kept as an explicit list rather than derived from the
// catalog so the ruling itself is reviewable here: a route leaving this list
// is a policy decision someone has to delete a line to make.
var newlyGuardedMutators = []string{
	"POST /emulation/viewport",
	"POST /emulation/geolocation",
	"POST /emulation/offline",
	"POST /emulation/headers",
	"POST /emulation/credentials",
	"POST /emulation/media",
	"POST /cookies",
	"POST /storage",
	"DELETE /storage",
	"POST /fingerprint/rotate",
	"POST /network/route",
	"DELETE /network/route",
	"POST /state/load",
	"GET /download",
}

// handoffRefusalReference is an endpoint that carried the handoff-pause guard
// before the ruling, so every newly-guarded endpoint can be compared against a
// refusal the product already shipped rather than against a literal restated in
// this file.
const handoffRefusalReference = "POST /evaluate"

func endpointByRoute(t *testing.T, route string) routes.Endpoint {
	t.Helper()
	for _, ep := range routes.Core() {
		if ep.Route() == route {
			return ep
		}
	}
	t.Fatalf("no catalog endpoint for %q", route)
	return routes.Endpoint{}
}

// TestNewlyGuardedMutatorsRefuseAPausedTabIdentically is the ruling's evidence:
// each endpoint moved onto the guard answers a paused tab with byte-identical
// status and body to an endpoint that was already guarded. Comparing whole
// envelopes rather than just the code is what makes "same error shape" a claim
// the test can actually break on — a divergent hint, remedy or status fails.
func TestNewlyGuardedMutatorsRefuseAPausedTabIdentically(t *testing.T) {
	paused := func() *guardProbeBridge {
		return &guardProbeBridge{paused: true, currentURL: "https://allowed.example/"}
	}

	refW, _ := recordProbeRoute(t, paused(), allCapabilities(t, t.TempDir()), endpointByRoute(t, handoffRefusalReference))
	assertHandoffRefusal(t, refW)
	wantStatus, wantBody := refW.Code, refW.Body.String()

	for _, route := range newlyGuardedMutators {
		t.Run(route, func(t *testing.T) {
			w, _ := recordProbeRoute(t, paused(), allCapabilities(t, t.TempDir()), endpointByRoute(t, route))

			assertHandoffRefusal(t, w)
			if w.Code != wantStatus || w.Body.String() != wantBody {
				t.Fatalf("refusal differs from %s\n got %d %s\nwant %d %s",
					handoffRefusalReference, w.Code, w.Body.String(), wantStatus, wantBody)
			}
		})
	}

	if len(newlyGuardedMutators) == 0 {
		t.Fatal("the ruled list is empty, so this test proves nothing")
	}
}

// TestReadProbesInRuledFamiliesStayUnguarded is the other half of the ruling:
// the read-only siblings inside the families it names keep serving a paused tab.
// Without it, widening a shared helper's guard set would silently block reads
// and nothing would notice.
func TestReadProbesInRuledFamiliesStayUnguarded(t *testing.T) {
	readProbes := []string{
		"GET /cookies",
		"GET /storage",
		"GET /state",
		"POST /state/save",
		"GET /network",
		"GET /network/stream",
		"GET /network/export",
		"GET /network/{requestId}",
	}

	for _, route := range readProbes {
		t.Run(route, func(t *testing.T) {
			b := &guardProbeBridge{paused: true, currentURL: "https://allowed.example/"}
			code, status, _ := probeRoute(t, b, allCapabilities(t, t.TempDir()), endpointByRoute(t, route))

			if code == handoffPausedCode {
				t.Fatalf("read probe now refuses a paused tab (%d %s); the ruling exempts reads and the catalog line says so", status, code)
			}
		})
	}
}

// TestRootStorageMethodDispatchSplitsTheGuard covers the one place this ruling
// could not express itself as a catalog line alone: GET, POST and DELETE
// /storage are three declarations served by one root handler over one shared
// body, so the guard set has to travel with the operation. The reads must still
// serve a paused tab while the writes refuse it — a widened shared helper would
// silently block reads and every per-route probe above would still pass, since
// they drive the tab form.
func TestRootStorageMethodDispatchSplitsTheGuard(t *testing.T) {
	cases := []struct {
		method     string
		body       string
		wantRefuse bool
	}{
		{http.MethodGet, "", false},
		{http.MethodPost, `{"type":"local","key":"probe","value":"1"}`, true},
		{http.MethodDelete, `{"type":"local","key":"probe"}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			h := New(&guardProbeBridge{paused: true, currentURL: "https://allowed.example/"}, allCapabilities(t, t.TempDir()), nil, nil, nil)

			req := httptest.NewRequest(tc.method, "/storage?tabId=tab1&type=local&key=probe", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleStorage(w, req)

			var resp struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			refused := resp.Code == handoffPausedCode

			if refused != tc.wantRefuse {
				t.Fatalf("%s /storage on a paused tab: refused=%v want %v (%d %s)", tc.method, refused, tc.wantRefuse, w.Code, w.Body.String())
			}
		})
	}
}

func TestRootStorageRefusesADialogBlockedTabForEveryMethod(t *testing.T) {
	cases := []struct {
		method string
		body   string
	}{
		{http.MethodGet, ""},
		{http.MethodPost, `{"type":"local","key":"probe","value":"1"}`},
		{http.MethodDelete, `{"type":"local","key":"probe"}`},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			b := &guardProbeBridge{currentURL: "https://allowed.example/", dialogs: pendingDialogManager()}
			h := New(b, allCapabilities(t, t.TempDir()), nil, nil, nil)

			req := httptest.NewRequest(tc.method, "/storage?tabId=tab1&type=local&key=probe", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleStorage(w, req)

			var resp struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Code != dialogBlockedCode {
				t.Fatalf("%s /storage on a dialog-blocked tab answered %d %s, want %s", tc.method, w.Code, w.Body.String(), dialogBlockedCode)
			}
		})
	}
}

func TestFixedDurationWaitRefusesADialogBlockedTab(t *testing.T) {
	for _, target := range []string{"/wait", "/tabs/tab1/wait"} {
		t.Run(target, func(t *testing.T) {
			b := &guardProbeBridge{currentURL: "https://allowed.example/", dialogs: pendingDialogManager()}
			h := New(b, allCapabilities(t, t.TempDir()), nil, nil, nil)
			mux := http.NewServeMux()
			h.RegisterRoutes(mux, nil)

			req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"tabId":"tab1","ms":50}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			var resp struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Code != dialogBlockedCode {
				t.Fatalf("POST %s {ms} on a dialog-blocked tab answered %d %s, want %s", target, w.Code, w.Body.String(), dialogBlockedCode)
			}
		})
	}
}

func TestFixedDurationWaitStillSleepsWithoutAPendingDialog(t *testing.T) {
	b := &guardProbeBridge{currentURL: "https://allowed.example/"}
	h := New(b, allCapabilities(t, t.TempDir()), nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/wait", strings.NewReader(`{"tabId":"tab1","ms":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleWait(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"waited":true`) {
		t.Fatalf("POST /wait {ms} without a dialog answered %d %s, want 200 waited", w.Code, w.Body.String())
	}
}

var mustDeclareDialogGuard = []string{
	"POST /navigate", "POST /back", "POST /forward", "POST /reload",
	"GET /snapshot", "GET /screenshot", "GET /annotate", "GET /capture", "GET /text", "GET /title", "GET /url", "GET /html", "GET /styles",
	"GET /value", "GET /attr", "GET /count", "GET /box", "GET /visible", "GET /enabled", "GET /checked",
	"GET /pdf", "POST /pdf", "GET /timing", "GET /a11y/audit", "GET /memory", "POST /memory/snapshot",
	"POST /action", "POST /actions", "POST /macro", "POST /wait", "POST /find", "POST /extract", "POST /evaluate",
	"POST /upload", "POST /solve", "POST /solve/{name}", "GET /download",
	"POST /emulation/viewport", "POST /emulation/geolocation", "POST /emulation/offline",
	"POST /emulation/headers", "POST /emulation/credentials", "POST /emulation/media",
	"POST /fingerprint/rotate",
	"GET /storage", "POST /storage", "DELETE /storage",
	"GET /state", "POST /state/save", "POST /state/load",
}

var dialogGuardAllowReasons = map[string]bool{
	"dialog":       true,
	"frame":        true,
	"tabs":         true,
	"handoff":      true,
	"own-tab":      true,
	"browser-side": true,
	"buffer":       true,
	"server-side":  true,
	"screencast":   true,
}

var dialogGuardAllowList = map[string]string{
	"POST /dialog":     "dialog",
	"GET /frame":       "frame",
	"POST /frame":      "frame",
	"POST /tab":        "tabs",
	"POST /close":      "tabs",
	"POST /lock":       "tabs",
	"POST /unlock":     "tabs",
	"POST /handoff":    "handoff",
	"POST /resume":     "handoff",
	"GET /handoff":     "handoff",
	"POST /audit/page": "own-tab",
	"POST /audit":      "own-tab",
	"POST /scrape":     "own-tab",
	"GET /cookies":     "browser-side",
	"POST /cookies":    "browser-side",
	"DELETE /cookies":  "browser-side",
	"GET /metrics":     "browser-side",
	"GET /memory/snapshot/{snapshotId}/summary": "server-side",
	"GET /memory/compare":                       "server-side",
	"GET /network/route":                        "browser-side",
	"POST /network/route":                       "browser-side",
	"DELETE /network/route":                     "browser-side",
	"POST /cache/clear":                         "browser-side",
	"GET /cache/status":                         "browser-side",
	"GET /stealth/status":                       "browser-side",
	"GET /solvers":                              "browser-side",
	"GET /config/autosolver":                    "browser-side",
	"GET /screencast/tabs":                      "browser-side",
	"GET /record/status":                        "browser-side",
	"GET /state/list":                           "browser-side",
	"GET /state/show":                           "browser-side",
	"DELETE /state":                             "browser-side",
	"POST /state/clean":                         "browser-side",
	"GET /network":                              "buffer",
	"GET /network/stream":                       "buffer",
	"GET /network/export":                       "buffer",
	"GET /network/export/stream":                "buffer",
	"GET /network/{requestId}":                  "buffer",
	"POST /network/clear":                       "buffer",
	"GET /console":                              "buffer",
	"POST /console/clear":                       "buffer",
	"GET /errors":                               "buffer",
	"POST /errors/clear":                        "buffer",
	"GET /clipboard/read":                       "server-side",
	"POST /clipboard/write":                     "server-side",
	"POST /clipboard/copy":                      "server-side",
	"GET /clipboard/paste":                      "server-side",
	"GET /screencast":                           "screencast",
	"POST /record/start":                        "screencast",
	"POST /record/stop":                         "screencast",
}

func TestEveryBindingIsPlacedForTheDialogGuard(t *testing.T) {
	must := map[string]bool{}
	for _, route := range mustDeclareDialogGuard {
		must[route] = true
		if _, listed := dialogGuardAllowList[route]; listed {
			t.Errorf("%s is both in the must-declare set and the allow-list", route)
		}
	}
	seen := map[string]bool{}
	for _, b := range (*Handlers)(nil).bridgeBindings() {
		seen[b.pattern] = true
		declared := b.guards&guardDialogBlocked != 0
		reason, allowed := dialogGuardAllowList[b.pattern]
		switch {
		case must[b.pattern] && !declared:
			t.Errorf("%s drives the page through CDP but does not declare guardDialogBlocked", b.pattern)
		case allowed && declared:
			t.Errorf("%s declares guardDialogBlocked but is allow-listed (%s); move it to the must-declare set", b.pattern, reason)
		case allowed && !dialogGuardAllowReasons[reason]:
			t.Errorf("%s is allow-listed with %q, which places nothing; give it one of the reasons in dialogGuardAllowReasons or move it to the must-declare set", b.pattern, reason)
		case !must[b.pattern] && !allowed:
			t.Errorf("%s is in neither the must-declare set nor the allow-list; place it with a one-word reason", b.pattern)
		}
	}
	for route := range must {
		if !seen[route] {
			t.Errorf("must-declare route %s has no binding", route)
		}
	}
	for route := range dialogGuardAllowList {
		if !seen[route] {
			t.Errorf("allow-listed route %s has no binding", route)
		}
	}
}

type dialogMidBatchBridge struct {
	mockBridge
	dialogs *bridgetabs.DialogManager
}

func (b *dialogMidBatchBridge) GetDialogManager() *bridgetabs.DialogManager { return b.dialogs }

func (b *dialogMidBatchBridge) ExecuteAction(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
	b.dialogs.SetPending("tab1", &bridge.DialogState{Type: "alert", Message: "opened by step 1"})
	return map[string]any{"ok": true}, nil
}

func TestActionsBatchStopsOnADialogOpenedMidRun(t *testing.T) {
	b := &dialogMidBatchBridge{dialogs: bridgetabs.NewDialogManager()}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	body := `{"actions":[{"kind":"press","key":"Enter"},{"kind":"press","key":"Enter"},{"kind":"press","key":"Enter"}]}`
	req := httptest.NewRequest("POST", "/actions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleActions(w, req)

	var resp struct {
		Results []actionResult `json:"results"`
		Total   int            `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v\n%s", err, w.Body.String())
	}
	if w.Code != http.StatusOK || len(resp.Results) != 2 || resp.Total != 3 {
		t.Fatalf("status=%d results=%d total=%d, want 200 with the run stopped after the second step\n%s", w.Code, len(resp.Results), resp.Total, w.Body.String())
	}
	if !resp.Results[0].Success {
		t.Errorf("step 1 should have run: %+v", resp.Results[0])
	}
	second := resp.Results[1]
	if second.Success || second.Code != dialogBlockedCode || second.Details["remedy"] == nil || !strings.Contains(second.Error, "opened by step 1") {
		t.Errorf("step 2 = %+v, want a failed step carrying %s with the remedy", second, dialogBlockedCode)
	}
}

func TestDeclaredLeaseGuardMatchesTheRefusal(t *testing.T) {
	checked, refusals := 0, 0
	for _, row := range guardedBindings(t) {
		t.Run(row.ep.Route(), func(t *testing.T) {
			b := &guardProbeBridge{currentURL: "https://allowed.example/", lockOwner: "someone-else"}
			code, status, answered := probeRoute(t, b, allCapabilities(t, t.TempDir()), row.ep)
			refused := code == "tab_locked" && status == http.StatusLocked
			declared := row.b.guards&guardTabLease != 0

			switch {
			case declared && !refused:
				t.Errorf("declares guardTabLease but a tab leased to another owner answered %d %q; either the declaration is wrong or the route needs a guardProbes entry that reaches its guards", status, code)
			case !declared && refused && answered:
				t.Errorf("refuses a leased tab without declaring guardTabLease, so its catalog line understates what it enforces")
			}
			checked++
			if refused {
				refusals++
			}
		})
	}
	if checked == 0 || refusals == 0 {
		t.Fatalf("checked %d routes and saw %d lease refusals; with none observed this test cannot tell a guard from its absence", checked, refusals)
	}
}

func TestALeasedTabRefusesEveryOtherOwnerOnLeaseGuardedRoutes(t *testing.T) {
	guarded := 0
	for _, row := range guardedBindings(t) {
		if row.b.guards&guardTabLease == 0 {
			continue
		}
		guarded++
		t.Run(row.ep.Route(), func(t *testing.T) {
			cfg := allCapabilities(t, t.TempDir())
			b := &guardProbeBridge{currentURL: "https://allowed.example/", lockOwner: "agent-a"}

			w, _ := recordProbeRouteAs(t, b, cfg, row.ep, "agent-b")
			var resp struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if w.Code != http.StatusLocked || resp.Code != "tab_locked" || !strings.Contains(resp.Error, "agent-a") {
				t.Errorf("owner agent-b on a tab leased by agent-a answered %d %q %q, want 423 tab_locked naming agent-a", w.Code, resp.Code, resp.Error)
			}

			w, _ = recordProbeRouteAs(t, b, cfg, row.ep, "agent-a")
			if w.Code == http.StatusLocked {
				t.Errorf("the lease holder agent-a was refused with 423: %s", w.Body.String())
			}
		})
	}
	for _, route := range []string{"POST /navigate", "POST /evaluate", "GET /cookies", "POST /cookies", "GET /storage", "POST /storage", "DELETE /storage", "POST /dialog"} {
		if !leaseDeclared(t, route) {
			t.Errorf("%s does not declare guardTabLease; leases must stop other agents changing the tab", route)
		}
	}
	if guarded == 0 {
		t.Fatal("no binding declares guardTabLease")
	}
}

func leaseDeclared(t *testing.T, route string) bool {
	t.Helper()
	for _, row := range guardedBindings(t) {
		if row.ep.Route() == route {
			return row.b.guards&guardTabLease != 0
		}
	}
	t.Fatalf("%s is not in the route catalog", route)
	return false
}

type leaseBatchBridge struct {
	mockBridge
	ranOn []string
}

func (b *leaseBatchBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	if tabID == "" {
		tabID = "tab1"
	}
	return bridge.NewTabHandle(context.Background()), tabID, nil
}

func (b *leaseBatchBridge) TabLockInfo(tabID string) *bridge.LockInfo {
	if tabID == "tab2" {
		return &bridge.LockInfo{Owner: "someone-else"}
	}
	return nil
}

func (b *leaseBatchBridge) ExecuteAction(_ context.Context, _ string, req bridge.ActionRequest) (map[string]any, error) {
	b.ranOn = append(b.ranOn, req.TabID)
	return map[string]any{"ok": true}, nil
}

func TestActionsBatchLeaseRefusalDoesNotSwitchLaterStepsOntoTheLockedTab(t *testing.T) {
	b := &leaseBatchBridge{}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	body := `{"actions":[{"kind":"press","key":"Enter","tabId":"tab2"},{"kind":"press","key":"Enter"}]}`
	req := httptest.NewRequest("POST", "/actions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleActions(w, req)

	var resp struct {
		Results []actionResult `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v\n%s", err, w.Body.String())
	}
	if len(resp.Results) != 2 || resp.Results[0].Success || !strings.Contains(resp.Results[0].Error, "locked by someone-else") {
		t.Fatalf("want step 1 refused by the lease, got %+v", resp.Results)
	}
	for _, tab := range b.ranOn {
		if tab == "tab2" {
			t.Fatalf("a later step ran on the leased tab: ranOn=%v", b.ranOn)
		}
	}
}
