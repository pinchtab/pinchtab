package orchestrator

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/session"
)

// alwaysAlive replaces processAliveFunc so mockCmd-backed instances satisfy
// instanceIsActive without depending on real process state.
func alwaysAlive(t *testing.T) {
	t.Helper()
	orig := processAliveFunc
	processAliveFunc = func(int) bool { return true }
	t.Cleanup(func() { processAliveFunc = orig })
}

// newBackendInstance spins up an httptest server that records the request
// path it received, registers it on the orchestrator under instanceID, and
// returns a teardown.
func newBackendInstance(t *testing.T, o *Orchestrator, instanceID string) (*httptest.Server, *string) {
	t.Helper()
	gotPath := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	o.client = srv.Client()
	inst := &InstanceInternal{
		Instance: bridge.Instance{ID: instanceID, Status: "running", URL: srv.URL},
		URL:      srv.URL,
		cmd:      &mockCmd{pid: 1, isAlive: true},
	}
	o.instances[instanceID] = inst
	o.syncInstanceToManager(&inst.Instance)
	return srv, gotPath
}

func TestRouteForRequest_AutoLaunchesRequestedBrowserTarget(t *testing.T) {
	alwaysAlive(t)
	stubPortAvailability(t, func(int) bool { return true })

	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"ok"}`))),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/navigate?browser=cloak", nil)

	target, status, err := o.RouteForRequest(req)
	if err != nil {
		t.Fatalf("RouteForRequest status=%d err=%v", status, err)
	}
	if target == "" {
		t.Fatal("target URL is empty")
	}
	instances := o.List()
	if len(instances) != 1 {
		t.Fatalf("instances = %d, want 1: %+v", len(instances), instances)
	}
	if instances[0].Target != "cloak" {
		t.Fatalf("BrowserTarget = %q, want cloak", instances[0].Target)
	}
	if instances[0].BrowserProvider != config.BrowserCloak {
		t.Fatalf("BrowserProvider = %q, want cloak", instances[0].BrowserProvider)
	}
	if instances[0].ProfileName != "default-cloak" {
		t.Fatalf("ProfileName = %q, want default-cloak", instances[0].ProfileName)
	}
}

func TestWrapShorthand_TabOwnerWins(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	_, pathA := newBackendInstance(t, o, "inst_a")
	_, pathB := newBackendInstance(t, o, "inst_b")

	o.instanceMgr.Locator.Register("tab-x", "inst_b")

	called := false
	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(599)
	})

	body := []byte(`{"tabId":"tab-x"}`)
	r := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))
	w := httptest.NewRecorder()

	wrapped(w, r)

	if called {
		t.Fatal("fallback should not have been called when tab owner is known")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *pathA != "" {
		t.Fatalf("instance A should not have been hit, got path %q", *pathA)
	}
	if *pathB != "/navigate" {
		t.Fatalf("instance B path = %q, want /navigate", *pathB)
	}
}

func TestWrapShorthand_TabOwnerBrowserTargetConflict(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})
	_, pathB := newBackendInstance(t, o, "inst_b")
	o.instances["inst_b"].Target = "chrome"
	o.instances["inst_b"].BrowserProvider = config.BrowserChrome
	o.syncInstanceToManager(&o.instances["inst_b"].Instance)
	o.instanceMgr.Locator.Register("tab-x", "inst_b")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("fallback should not run for tab-owner conflict")
	})

	r := httptest.NewRequest("POST", "/navigate?browser=cloak&tabId=tab-x", nil)
	w := httptest.NewRecorder()

	wrapped(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 body=%s", w.Code, w.Body.String())
	}
	if *pathB != "" {
		t.Fatalf("conflicting target should not proxy to backend, got path %q", *pathB)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("browser_conflict")) {
		t.Fatalf("body = %s, want browser_conflict", w.Body.String())
	}
}

func TestWrapShorthand_TabNotFoundMultipleInstancesIs404(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(t)
	newBackendInstance(t, o, "inst_a")
	newBackendInstance(t, o, "inst_b")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("fallback must not run when explicit tab id misses with multiple instances")
	})

	r := httptest.NewRequest("GET", "/text?tabId=ghost", nil)
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestWrapShorthand_TabNotFoundSingleInstanceFallsThrough(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(t)
	_, gotPath := newBackendInstance(t, o, "inst_only")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("legacy ergonomics: orchestrator should proxy directly, not call fallback")
	})

	r := httptest.NewRequest("GET", "/text?tabId=ghost", nil)
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *gotPath != "/text" {
		t.Fatalf("backend path = %q, want /text", *gotPath)
	}
}

func TestWrapShorthand_BrowserTargetBypassesMismatchedIdentityBinding(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "chrome",
		Targets: config.BrowserTargetsConfig{
			"chrome": {Provider: config.BrowserChrome},
			"cloak":  {Provider: config.BrowserCloak},
		},
	})
	_, pathA := newBackendInstance(t, o, "inst_a")
	o.instances["inst_a"].Target = "chrome"
	o.instances["inst_a"].BrowserProvider = config.BrowserChrome
	o.bindings.BindAgent("agent-1", "inst_a")

	fallbackCalled := false
	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalled = true
		w.WriteHeader(http.StatusAccepted)
	})

	r := httptest.NewRequest("GET", "/text?browser=cloak", nil)
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 body=%s", w.Code, w.Body.String())
	}
	if !fallbackCalled {
		t.Fatal("fallback should run so browserTarget selection can choose the requested target")
	}
	if *pathA != "" {
		t.Fatalf("mismatched identity binding should not proxy to backend, got path %q", *pathA)
	}
}

func TestWrapShorthand_SessionBindingHonoredOnTrustedHop(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	newBackendInstance(t, o, "inst_a")
	_, pathB := newBackendInstance(t, o, "inst_b")

	o.bindings.BindSession("ses_1", "inst_b")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("session binding should have routed; fallback must not run")
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderPTSessionID, "ses_1")
	r = r.WithContext(handlers.MarkTrustedInternalProxy(r.Context()))
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *pathB != "/text" {
		t.Fatalf("instance B path = %q, want /text", *pathB)
	}
}

func TestWrapShorthand_SessionBindingHonoredFromAuthenticatedContext(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	newBackendInstance(t, o, "inst_a")
	_, pathB := newBackendInstance(t, o, "inst_b")

	o.bindings.BindSession("ses_public", "inst_b")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("session binding should have routed; fallback must not run")
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r = session.WithSession(r, &session.Session{ID: "ses_public", AgentID: "agent-1"})
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *pathB != "/text" {
		t.Fatalf("instance B path = %q, want /text", *pathB)
	}
}

func TestWrapShorthand_SessionHeaderIgnoredWithoutTrust(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	newBackendInstance(t, o, "inst_a")
	newBackendInstance(t, o, "inst_b")

	o.bindings.BindSession("ses_1", "inst_b")

	called := false
	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})

	// No trusted-internal marker — header is internal metadata, must be ignored.
	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderPTSessionID, "ses_1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if !called {
		t.Fatal("fallback must run when session header is untrusted")
	}
	if w.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (fallback signal)", w.Code)
	}
}

func TestWrapShorthand_AgentBindingHonored(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	newBackendInstance(t, o, "inst_a")
	_, pathB := newBackendInstance(t, o, "inst_b")

	o.bindings.BindAgent("agent-1", "inst_b")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("agent binding should have routed; fallback must not run")
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *pathB != "/text" {
		t.Fatalf("instance B path = %q, want /text", *pathB)
	}
}

func TestWrapShorthand_StaleBindingFallsThrough(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())
	_, pathA := newBackendInstance(t, o, "inst_a")

	// Bind to an instance that no longer exists.
	o.bindings.BindAgent("agent-1", "inst_gone")

	called := false
	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Simulate the strategy fallback proxying to the only running instance.
		o.ProxyToTarget(w, r, o.instances["inst_a"].URL+r.URL.Path)
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if !called {
		t.Fatal("stale binding should fall through to fallback")
	}
	if *pathA != "/text" {
		t.Fatalf("fallback path on instance A = %q, want /text", *pathA)
	}
	// The successful fallback proxy rebinds the agent to the instance that
	// actually served the request (inst_a), replacing the stale entry.
	if got, ok := o.bindings.ResolveAgent("agent-1"); !ok || got != "inst_a" {
		t.Fatalf("agent binding after stale fallthrough = %q, %v; want inst_a, true", got, ok)
	}
}
