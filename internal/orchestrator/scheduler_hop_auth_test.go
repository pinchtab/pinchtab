package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/scheduler"
)

type recordingRecorder struct {
	mu     sync.Mutex
	events []activity.Event
}

func (r *recordingRecorder) Enabled() bool { return true }
func (r *recordingRecorder) Record(e activity.Event) error {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
	return nil
}
func (r *recordingRecorder) Query(activity.Filter) ([]activity.Event, error) { return nil, nil }
func (r *recordingRecorder) last() (activity.Event, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		return activity.Event{}, false
	}
	return r.events[len(r.events)-1], true
}

// A scheduler-executed action must record as 'scheduler', not 'client': the executor routes
// hop auth through the orchestrator (AuthorizeTabRequest → applyInstanceAuth), which sends the
// internal token on a trusted child hop, so the X-PinchTab-Source/Tab-Id survive the instance's
// real TrustedInternalProxyStripMiddleware. Built the way server.go builds it — an
// orchestrator-generated token, no PINCHTAB_INTERNAL_TOKEN in the environment. Removing the
// authorizer call (the executor's e.resolver.(RequestAuthorizer) branch) strips the headers and
// records 'client', which is the pre-fix behavior this proves is gone.
func TestSchedulerActionRecordsAsSchedulerThroughRealIngress(t *testing.T) {
	rec := &recordingRecorder{}
	var gotSource, gotTabID string
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSource = r.Header.Get(activity.HeaderPTSource)
		gotTabID = r.Header.Get(activity.HeaderPTTabID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	o := &Orchestrator{
		internalToken: "orch-generated-token",
		instances:     map[string]*InstanceInternal{},
		instanceMgr:   instance.NewManager(nil),
	}
	srv := httptest.NewServer(
		handlers.TrustedInternalProxyStripMiddleware(o.internalToken)(
			activity.Middleware(rec, "fallback", final)))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	inst := &bridge.Instance{ID: "inst-1", Port: parsed.Port(), Status: "running"}
	o.instanceMgr.Repo.Add(inst)
	o.instanceMgr.Locator.Register("tab-42", "inst-1")
	o.instances["inst-1"] = &InstanceInternal{Instance: *inst}

	exec := scheduler.NewActionExecutor(o)
	if _, err := exec.Execute(context.Background(), &scheduler.Task{Action: "click", Ref: "e5", TabID: "tab-42"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotSource != "scheduler" {
		t.Errorf("inbound X-PinchTab-Source = %q, want scheduler (hop auth must let it survive real ingress)", gotSource)
	}
	if gotTabID != "tab-42" {
		t.Errorf("inbound X-PinchTab-Tab-Id = %q, want tab-42 (the tab id header must survive ingress)", gotTabID)
	}
	evt, ok := rec.last()
	if !ok {
		t.Fatal("no activity event recorded")
	}
	if evt.Source != "scheduler" {
		t.Errorf("recorded activity source = %q, want scheduler", evt.Source)
	}
	if evt.TabID != "tab-42" {
		t.Errorf("recorded activity tab id = %q, want tab-42", evt.TabID)
	}
}
