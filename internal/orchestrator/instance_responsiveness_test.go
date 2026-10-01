package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

const stubInstanceID = "inst_probe"

func hangUntilClientGivesUp(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }

func answerHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","tabs":0}`))
}

func answerTabs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"tabs":[]}`))
}

func shortProbeBudget(o *Orchestrator) time.Duration {
	o.timings.responsivenessProbeBudget = 150 * time.Millisecond
	return o.timings.responsivenessProbeBudget
}

func orchestratorOverStubChild(t *testing.T, handler http.Handler) (*Orchestrator, *mockRunner) {
	t.Helper()
	backend := httptest.NewServer(handler)
	t.Cleanup(backend.Close)
	runner := &mockRunner{portAvail: true}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.client = backend.Client()
	o.mu.Lock()
	o.instances[stubInstanceID] = &InstanceInternal{
		Instance: bridge.Instance{ID: stubInstanceID, ProfileName: "p1", Status: "running"},
		URL:      backend.URL,
	}
	o.mu.Unlock()
	return o, runner
}

func stubChild(health, tabs http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/tabs", tabs)
	return mux
}

func TestResponsivenessIsThreeDistinctValuesNeverSharingARepresentation(t *testing.T) {
	values := map[string]bool{
		bridge.ResponsivenessResponsive:   true,
		bridge.ResponsivenessUnresponsive: true,
		bridge.ResponsivenessUnknown:      true,
	}
	if len(values) != 3 {
		t.Fatalf("responsiveness values collide: %v", values)
	}
	for _, v := range []string{"", "garbage"} {
		if got := bridge.NormalizeResponsiveness(v); got != bridge.ResponsivenessUnknown {
			t.Fatalf("NormalizeResponsiveness(%q) = %q, want unknown", v, got)
		}
	}
}

func TestAnInstanceAnsweringHealthButNotTabsIsUnresponsiveAndStillRunning(t *testing.T) {
	o, runner := orchestratorOverStubChild(t, stubChild(answerHealth, hangUntilClientGivesUp))
	shortProbeBudget(o)
	o.mu.RLock()
	before := o.instances[stubInstanceID]
	o.mu.RUnlock()

	if got := o.List()[0].Responsiveness; got != bridge.ResponsivenessUnknown {
		t.Fatalf("never-probed instance reads %q, want unknown", got)
	}
	o.RefreshCrashes()

	inst := o.List()[0]
	if inst.Responsiveness != bridge.ResponsivenessUnresponsive {
		t.Fatalf("responsiveness = %q, want unresponsive", inst.Responsiveness)
	}
	if inst.Status != "running" {
		t.Fatalf("status = %q, want running: responsiveness never changes routing", inst.Status)
	}
	o.mu.RLock()
	after := o.instances[stubInstanceID]
	o.mu.RUnlock()
	if runner.runCalled || after != before {
		t.Fatalf("an unresponsive result restarted the instance (runCalled=%v, replaced=%v)", runner.runCalled, after != before)
	}
}

func TestAnInstanceBlockingEveryRouteIsUnknownNotUnresponsive(t *testing.T) {
	o, _ := orchestratorOverStubChild(t, stubChild(hangUntilClientGivesUp, hangUntilClientGivesUp))
	shortProbeBudget(o)
	o.RefreshCrashes()
	if got := o.List()[0].Responsiveness; got != bridge.ResponsivenessUnknown {
		t.Fatalf("responsiveness = %q, want unknown: its /health never answered either", got)
	}
}

func TestAnInstanceAnsweringBothRoutesIsResponsive(t *testing.T) {
	o, _ := orchestratorOverStubChild(t, stubChild(answerHealth, answerTabs))
	shortProbeBudget(o)
	o.RefreshCrashes()
	if got := o.List()[0].Responsiveness; got != bridge.ResponsivenessResponsive {
		t.Fatalf("responsiveness = %q, want responsive", got)
	}
}

func TestAStoppedInstanceReadsUnknownAgainAfterTheNextRefresh(t *testing.T) {
	o, _ := orchestratorOverStubChild(t, stubChild(answerHealth, answerTabs))
	shortProbeBudget(o)
	o.RefreshCrashes()
	o.mu.Lock()
	o.instances[stubInstanceID].Status = "stopped"
	o.mu.Unlock()
	o.RefreshCrashes()
	if got := o.List()[0].Responsiveness; got != bridge.ResponsivenessUnknown {
		t.Fatalf("responsiveness = %q after the instance stopped, want unknown", got)
	}
}

func TestTheProbeBudgetBoundsTheCrashSummaryLatency(t *testing.T) {
	o, _ := orchestratorOverStubChild(t, stubChild(answerHealth, hangUntilClientGivesUp))
	budget := shortProbeBudget(o)
	started := time.Now()
	o.CrashSummary()
	elapsed := time.Since(started)
	if elapsed < budget || elapsed > budget+500*time.Millisecond {
		t.Fatalf("CrashSummary took %v with one hanging child, want within the %v probe budget plus a small margin", elapsed, budget)
	}
}

type capturingRecorder struct {
	mu     sync.Mutex
	events []activity.Event
}

func (c *capturingRecorder) Enabled() bool { return true }
func (c *capturingRecorder) Record(evt activity.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, evt)
	return nil
}
func (c *capturingRecorder) Query(activity.Filter) ([]activity.Event, error) { return nil, nil }

func TestTheTabsProbeReachesTheChildAsOrchestratorActivityNotClientActivity(t *testing.T) {
	rec := &capturingRecorder{}
	o, _ := orchestratorOverStubChild(t, activity.Middleware(rec, "client", stubChild(answerHealth, answerTabs)))
	shortProbeBudget(o)
	o.RefreshCrashes()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	paths := map[string]string{}
	for _, evt := range rec.events {
		paths[evt.Path] = evt.Source
	}
	for _, path := range []string{"/tabs", "/health"} {
		if source, ok := paths[path]; !ok || source != orchestratorActivitySource {
			t.Fatalf("probe of %s recorded with source %q (seen=%v), want %q so it counts as neither activity nor an idle reset", path, source, ok, orchestratorActivitySource)
		}
	}
}

func awaitRefresh(t *testing.T, o *Orchestrator) time.Time {
	t.Helper()
	o.refreshMu.Lock()
	done := o.refreshDone
	o.refreshMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the crash fan-out never finished")
		}
	}
	return time.Now()
}

func addStubInstance(o *Orchestrator, id, url string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.instances[id] = &InstanceInternal{
		Instance: bridge.Instance{ID: id, ProfileName: id, Status: "running"},
		URL:      url,
	}
}

func TestAHangingTabsProbeReleasesTheRequestAtTheJoinWaitAndRecordsItsVerdictLater(t *testing.T) {
	o, _ := orchestratorOverStubChild(t, stubChild(answerHealth, hangUntilClientGivesUp))
	o.timings.responsivenessProbeBudget = 800 * time.Millisecond

	started := time.Now()
	o.CrashSummary()
	elapsed := time.Since(started)
	if elapsed < refreshJoinWait || elapsed > refreshJoinWait+150*time.Millisecond || elapsed >= 500*time.Millisecond {
		t.Fatalf("CrashSummary took %v with one hanging child, want the %v join wait, under the CLI's 500ms presence check", elapsed, refreshJoinWait)
	}
	if got := o.List()[0].Responsiveness; got != bridge.ResponsivenessUnknown {
		t.Fatalf("responsiveness = %q before the probe finished, want unknown", got)
	}
	awaitRefresh(t, o)
	if got := o.List()[0].Responsiveness; got != bridge.ResponsivenessUnresponsive {
		t.Fatalf("responsiveness = %q after the probe finished, want unresponsive", got)
	}
}

func TestConcurrentRefreshesShareOneFanOut(t *testing.T) {
	var mu sync.Mutex
	tabsHits := 0
	o, _ := orchestratorOverStubChild(t, stubChild(answerHealth, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tabsHits++
		mu.Unlock()
		hangUntilClientGivesUp(w, r)
	}))
	o.timings.responsivenessProbeBudget = 800 * time.Millisecond

	o.RefreshCrashes()
	o.RefreshCrashes()
	awaitRefresh(t, o)
	mu.Lock()
	defer mu.Unlock()
	if tabsHits != 1 {
		t.Fatalf("two refreshes during one in-flight fan-out probed /tabs %d times, want 1", tabsHits)
	}
}

func TestTheFanOutProbesInstancesInParallel(t *testing.T) {
	budget := 300 * time.Millisecond
	o, _ := orchestratorOverStubChild(t, stubChild(answerHealth, hangUntilClientGivesUp))
	o.timings.responsivenessProbeBudget = budget
	o.mu.RLock()
	url := o.instances[stubInstanceID].URL
	o.mu.RUnlock()
	addStubInstance(o, "inst_probe_2", url)

	started := time.Now()
	o.RefreshCrashes()
	elapsed := awaitRefresh(t, o).Sub(started)
	if elapsed > budget+150*time.Millisecond {
		t.Fatalf("fan-out over two hanging instances took %v, want one %v budget: the probes ran serially", elapsed, budget)
	}
	for _, inst := range o.List() {
		if inst.Responsiveness != bridge.ResponsivenessUnresponsive {
			t.Fatalf("%s responsiveness = %q, want unresponsive", inst.ID, inst.Responsiveness)
		}
	}
}

func TestATabsProbeCutOffWithoutAnAnswerIsUnknownNotUnresponsive(t *testing.T) {
	o, _ := orchestratorOverStubChild(t, stubChild(answerHealth, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	shortProbeBudget(o)
	o.RefreshCrashes()
	awaitRefresh(t, o)
	if got := o.List()[0].Responsiveness; got != bridge.ResponsivenessUnknown {
		t.Fatalf("responsiveness = %q when /tabs dropped the connection, want unknown: only a probe that outlasts the budget is unresponsive", got)
	}
}
