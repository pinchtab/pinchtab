package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/session"
)

func TestSessionLifecycleHook_ClearsBinding(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.bindings.BindSession("ses_dead", "inst_a")

	hook := o.SessionLifecycleHook()
	hook(session.LifecycleEvent{SessionID: "ses_dead", AgentID: "agent-x", Reason: session.LifecycleReasonRevoked})
	o.sessionCloses.Wait()

	if _, ok := o.bindings.ResolveSession("ses_dead"); ok {
		t.Fatal("binding should have been cleared")
	}
}

func TestSessionLifecycleHook_NoopOnEmptyID(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.bindings.BindSession("ses_a", "inst_a")
	hook := o.SessionLifecycleHook()
	hook(session.LifecycleEvent{SessionID: "", Reason: session.LifecycleReasonExpired})
	o.sessionCloses.Wait()
	if _, ok := o.bindings.ResolveSession("ses_a"); !ok {
		t.Fatal("unrelated binding should not be touched")
	}
}

type recordedClose struct {
	path, sessionID string
	trusted         bool
}

type closeRecorder struct {
	mu    sync.Mutex
	calls []recordedClose
}

func (c *closeRecorder) handler(hang bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.calls = append(c.calls, recordedClose{
			path:      r.Method + " " + r.URL.Path,
			sessionID: r.Header.Get(activity.HeaderPTSessionID),
			trusted:   r.Header.Get(handlers.InternalTokenHeader) != "",
		})
		c.mu.Unlock()
		if hang {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte(`{"closed":[],"kept":[]}`))
	}
}

func (c *closeRecorder) snapshot() []recordedClose {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]recordedClose(nil), c.calls...)
}

func addInstanceOver(t *testing.T, o *Orchestrator, id string, handler http.Handler, authToken string) {
	t.Helper()
	backend := httptest.NewServer(handler)
	t.Cleanup(backend.Close)
	o.client = backend.Client()
	o.mu.Lock()
	o.instances[id] = &InstanceInternal{
		Instance:  bridge.Instance{ID: id, ProfileName: id, Status: "running"},
		URL:       backend.URL,
		authToken: authToken,
	}
	o.mu.Unlock()
}

func TestAnEndedSessionAsksEverySpawnedInstanceToCloseItsTabs(t *testing.T) {
	for _, reason := range []string{session.LifecycleReasonRevoked, session.LifecycleReasonExpired, session.LifecycleReasonPruned} {
		t.Run(reason, func(t *testing.T) {
			o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
			spawnedA, spawnedB, attached := &closeRecorder{}, &closeRecorder{}, &closeRecorder{}
			addInstanceOver(t, o, "inst_a", spawnedA.handler(false), "")
			addInstanceOver(t, o, "inst_b", spawnedB.handler(false), "")
			addInstanceOver(t, o, "inst_attached", attached.handler(false), "bridge-token")
			o.bindings.BindSession("ses_dead", "inst_a")

			o.SessionLifecycleHook()(session.LifecycleEvent{SessionID: "ses_dead", Reason: reason})
			o.sessionCloses.Wait()

			want := recordedClose{path: "POST " + handlers.SessionTabsClosePath, sessionID: "ses_dead", trusted: true}
			for name, rec := range map[string]*closeRecorder{"inst_a": spawnedA, "inst_b": spawnedB} {
				if calls := rec.snapshot(); len(calls) != 1 || calls[0] != want {
					t.Errorf("%s received %+v, want one %+v", name, calls, want)
				}
			}
			if calls := attached.snapshot(); len(calls) != 0 {
				t.Errorf("the attached bridge received %+v: it cannot honour the trusted hop", calls)
			}
			if _, ok := o.bindings.ResolveSession("ses_dead"); ok {
				t.Error("binding should have been cleared")
			}
		})
	}
}

func TestAHangingInstanceNeitherStallsTheHookNorOutlivesTheCloseBudget(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.timings.sessionTabsCloseBudget = 200 * time.Millisecond
	hanging := &closeRecorder{}
	addInstanceOver(t, o, "inst_hang", hanging.handler(true), "")

	started := time.Now()
	o.SessionLifecycleHook()(session.LifecycleEvent{SessionID: "ses_dead", Reason: session.LifecycleReasonRevoked})
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("the hook took %v: the close must run off the dispatch goroutine", elapsed)
	}
	o.sessionCloses.Wait()
	if elapsed := time.Since(started); elapsed < o.timings.sessionTabsCloseBudget || elapsed > o.timings.sessionTabsCloseBudget+300*time.Millisecond {
		t.Fatalf("the close took %v against a hanging instance, want the %v budget", elapsed, o.timings.sessionTabsCloseBudget)
	}
	if len(hanging.snapshot()) != 1 {
		t.Fatal("the hanging instance was never asked")
	}
}
