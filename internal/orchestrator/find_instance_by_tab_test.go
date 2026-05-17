package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestFindInstanceByTab_KnownTabReturnsInstance(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tabs" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tabs":[{"id":"tab-1","url":"about:blank"},{"id":"tab-2","url":"https://example.com"}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	o.client = srv.Client()

	inst := &InstanceInternal{
		Instance: bridge.Instance{
			ID:              "inst_a",
			Status:          "running",
			URL:             srv.URL,
			BrowserTarget:   "chrome-default",
			BrowserProvider: "chrome",
		},
		URL: srv.URL,
		cmd: &mockCmd{pid: 1, isAlive: true},
	}
	o.instances["inst_a"] = inst

	got, ok := o.FindInstanceByTab("tab-2")
	if !ok {
		t.Fatal("expected FindInstanceByTab to return ok for known tab")
	}
	if got == nil || got.ID != "inst_a" {
		t.Fatalf("expected instance inst_a, got %+v", got)
	}
	if got.BrowserTarget != "chrome-default" {
		t.Fatalf("expected BrowserTarget=chrome-default, got %q", got.BrowserTarget)
	}
}

func TestFindInstanceByTab_UnknownTabReturnsFalse(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestrator(t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tabs" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tabs":[{"id":"tab-1","url":"about:blank"}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	o.client = srv.Client()

	inst := &InstanceInternal{
		Instance: bridge.Instance{ID: "inst_a", Status: "running", URL: srv.URL},
		URL:      srv.URL,
		cmd:      &mockCmd{pid: 1, isAlive: true},
	}
	o.instances["inst_a"] = inst

	if got, ok := o.FindInstanceByTab("nope"); ok || got != nil {
		t.Fatalf("expected (nil,false) for unknown tab, got (%+v,%v)", got, ok)
	}
}

func TestFindInstanceByTab_EmptyTabID(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{})
	if got, ok := o.FindInstanceByTab(""); ok || got != nil {
		t.Fatalf("expected (nil,false) for empty tabID, got (%+v,%v)", got, ok)
	}
}
