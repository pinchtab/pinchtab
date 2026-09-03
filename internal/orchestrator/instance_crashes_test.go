package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func orchestratorOverInstanceHealth(t *testing.T, id, healthBody string) *Orchestrator {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(healthBody))
	}))
	t.Cleanup(backend.Close)
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.client = backend.Client()
	o.mu.Lock()
	o.instances[id] = &InstanceInternal{
		Instance: bridge.Instance{ID: id, ProfileName: "p1", Status: "running"},
		URL:      backend.URL,
	}
	o.mu.Unlock()
	return o
}

// Browser crashes are recorded by the process owning the browser, which in server
// mode is the instance; the front door has to ask. Its /health then merges every
// instance's record, naming the instance on each event, and the instance list
// carries the same block.
func TestTheFrontDoorLearnsAnInstancesCrashesFromItsHealth(t *testing.T) {
	const id = "inst_crashed"
	o := orchestratorOverInstanceHealth(t, id, `{"status":"ok","tabs":0,"crashes":{"total":2,"recent":[
		{"time":"2026-09-03T08:48:11Z","reason":"unexpected context cancellation"},
		{"time":"2026-09-03T08:49:47Z","reason":"unexpected context cancellation"}]}}`)

	summary := o.CrashSummary()
	if summary.Total != 2 || len(summary.Recent) != 2 {
		t.Fatalf("summary = %+v, want the instance's two crashes", summary)
	}
	for _, ev := range summary.Recent {
		if ev.InstanceID != id {
			t.Errorf("event %+v does not name its instance", ev)
		}
	}
	instances := o.List()
	if len(instances) != 1 || instances[0].Crashes == nil || instances[0].Crashes.Total != 2 {
		t.Errorf("List() = %+v, want the crash block on the instance", instances)
	}
}

func TestAnInstanceThatNeverCrashedCarriesNoCrashBlock(t *testing.T) {
	o := orchestratorOverInstanceHealth(t, "inst_clean", `{"status":"ok","tabs":1}`)
	if summary := o.CrashSummary(); summary.Total != 0 || len(summary.Recent) != 0 {
		t.Errorf("summary = %+v, want empty", summary)
	}
	if instances := o.List(); instances[0].Crashes != nil {
		t.Errorf("List() carries a crash block for a clean instance: %+v", instances[0].Crashes)
	}
}
