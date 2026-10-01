package orchestrator

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/session"
)

func instWithURL(id, url string) bridge.Instance {
	return bridge.Instance{ID: id, Status: "running", URL: url}
}

func TestWrapShorthand_AgentBindingWrittenOnSuccess(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)
	_, gotPath := newBackendInstance(t, o, "inst_only")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		// Simulate the strategy's plain proxy to the only instance.
		o.ProxyToTarget(w, r, o.instances["inst_only"].URL+r.URL.Path)
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *gotPath != "/text" {
		t.Fatalf("path = %q, want /text", *gotPath)
	}
	if got, ok := o.bindings.ResolveAgent("agent-1"); !ok || got != "inst_only" {
		t.Fatalf("agent binding = %q, %v; want inst_only, true", got, ok)
	}
}

func TestWrapShorthand_SessionBindingWrittenOnTrustedSuccess(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)
	_, gotPath := newBackendInstance(t, o, "inst_only")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		o.ProxyToTarget(w, r, o.instances["inst_only"].URL+r.URL.Path)
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderPTSessionID, "ses_1")
	r = r.WithContext(handlers.MarkTrustedInternalProxy(r.Context()))
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if *gotPath != "/text" {
		t.Fatalf("backend path = %q, want /text", *gotPath)
	}
	if got, ok := o.bindings.ResolveSession("ses_1"); !ok || got != "inst_only" {
		t.Fatalf("session binding = %q, %v; want inst_only, true", got, ok)
	}
}

func TestWrapShorthand_SessionBindingWrittenFromAuthenticatedContext(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)
	_, gotPath := newBackendInstance(t, o, "inst_only")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		o.ProxyToTarget(w, r, o.instances["inst_only"].URL+r.URL.Path)
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r = session.WithSession(r, &session.Session{ID: "ses_public", AgentID: "agent-1"})
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if *gotPath != "/text" {
		t.Fatalf("backend path = %q, want /text", *gotPath)
	}
	if got, ok := o.bindings.ResolveSession("ses_public"); !ok || got != "inst_only" {
		t.Fatalf("session binding = %q, %v; want inst_only, true", got, ok)
	}
}

func TestWrapShorthand_SessionBindingNotWrittenWithoutTrust(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)
	newBackendInstance(t, o, "inst_only")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		o.ProxyToTarget(w, r, o.instances["inst_only"].URL+r.URL.Path)
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderPTSessionID, "spoofer")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if _, ok := o.bindings.ResolveSession("spoofer"); ok {
		t.Fatal("untrusted session header must not write a binding")
	}
}

func TestWrapShorthand_TabOwnerRouteRebindsIdentityOnSuccess(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)
	newBackendInstance(t, o, "inst_a")
	_, pathB := newBackendInstance(t, o, "inst_b")

	o.bindings.BindAgent("agent-1", "inst_a")
	o.instanceMgr.RememberTab("tab-on-b", "inst_b")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("tab-owner routing should not fall back")
	})

	body := []byte(`{"tabId":"tab-on-b"}`)
	r := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if *pathB != "/navigate" {
		t.Fatalf("backend path = %q, want /navigate", *pathB)
	}
	if got, _ := o.bindings.ResolveAgent("agent-1"); got != "inst_b" {
		t.Fatalf("agent binding after cross-instance success = %q, want inst_b", got)
	}
}

func TestWrapShorthand_StrictRejectsCrossInstance(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)
	o.SetStrictCrossInstanceTab(true)
	newBackendInstance(t, o, "inst_a")
	_, pathB := newBackendInstance(t, o, "inst_b")

	o.bindings.BindAgent("agent-1", "inst_a")
	o.instanceMgr.RememberTab("tab-on-b", "inst_b")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("strict mode must reject before fallback")
	})

	body := []byte(`{"tabId":"tab-on-b"}`)
	r := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if *pathB != "" {
		t.Fatalf("instance B should not have been hit, got path %q", *pathB)
	}
	if got, _ := o.bindings.ResolveAgent("agent-1"); got != "inst_a" {
		t.Fatalf("strict mode must not move binding; got %q, want inst_a", got)
	}
}

func TestWrapShorthand_StrictAllowsSameInstance(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)
	o.SetStrictCrossInstanceTab(true)
	_, gotPath := newBackendInstance(t, o, "inst_a")

	o.bindings.BindAgent("agent-1", "inst_a")
	o.instanceMgr.RememberTab("tab-on-a", "inst_a")

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("tab-owner must succeed when binding matches owner")
	})

	r := httptest.NewRequest("GET", "/text?tabId=tab-on-a", nil)
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *gotPath != "/text" {
		t.Fatalf("path = %q, want /text", *gotPath)
	}
}

func TestWrapShorthand_BindingNotWrittenOnFailure(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(t.TempDir())
	alwaysAlive(o)

	// Backend that always fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	o.client = srv.Client()
	o.instances["inst_fail"] = &InstanceInternal{
		Instance: instWithURL("inst_fail", srv.URL),
		URL:      srv.URL,
		cmd:      &mockCmd{pid: 1, isAlive: true},
	}
	o.instanceMgr.TrackInstance(&o.instances["inst_fail"].Instance)

	wrapped := o.WrapShorthand(func(w http.ResponseWriter, r *http.Request) {
		o.ProxyToTarget(w, r, srv.URL+r.URL.Path)
	})

	r := httptest.NewRequest("GET", "/text", nil)
	r.Header.Set(activity.HeaderAgentID, "agent-1")
	w := httptest.NewRecorder()
	wrapped(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if _, ok := o.bindings.ResolveAgent("agent-1"); ok {
		t.Fatal("binding must not be written on failed proxy")
	}
}
