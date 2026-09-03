package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/proxy"
)

// recordedRun is what the three durable channels held after one multi-step run.
// They are collected together on purpose: the defect was that all three were
// silent at once, and fixing one of them would look like progress while a run in
// which every step failed still read as healthy traffic everywhere else.
type recordedRun struct {
	channels    map[string]string
	activity    map[string]any
	failures    []map[string]any
	failedDelta uint64
}

// runMultiStep drives the real chain — activity outside, logging inside, as
// internal/server stacks them — around the ONE shared finalizer both /actions and
// /macro reach. Macro cannot be exercised end to end with security.allowMacro off,
// which is why the surface is a parameter here rather than a second handler.
func runMultiStep(t *testing.T, path string, results []actionResult, extra map[string]any) recordedRun {
	t.Helper()

	logs := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	logDir := t.TempDir()
	store, err := activity.NewStore(logDir, 1)
	if err != nil {
		t.Fatalf("activity store: %v", err)
	}
	resetObservabilityForTests()

	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	route := &browserops.RouteMetadata{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.writeMultiStepActionResult(w, r, context.Background(), "tab1", results, len(results), route, extra)
	})

	chain := activity.Middleware(store, "client", LoggingMiddleware(handler))
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("X-PinchTab-Source", "client")
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("the shared writer answered %d; the 200-with-per-item-results contract is deliberate and this card must not change it", w.Code)
	}

	failed, _ := SnapshotMetrics()["requestsFailed"].(uint64)
	event := lastActivityEvent(t, logDir)
	encodedEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	failures := allRecentFailureEvents(t)
	encodedFailures, err := json.Marshal(failures)
	if err != nil {
		t.Fatal(err)
	}

	return recordedRun{
		channels: map[string]string{
			"the server log":      logs.String(),
			"the activity record": string(encodedEvent),
			"failures.recent":     string(encodedFailures),
		},
		activity:    event,
		failures:    failures,
		failedDelta: failed,
	}
}

// allRecentFailureEvents tolerates an empty list, unlike its sibling: the
// successful run must record nothing, and that is an assertion rather than a
// broken fixture.
func allRecentFailureEvents(t *testing.T) []map[string]any {
	t.Helper()
	snapshot := FailureSnapshot(LayerInstance)
	events, _ := snapshot["recent"].([]map[string]any)
	return events
}

func failedStep(index int, code, message string) actionResult {
	return actionResult{Index: index, Success: false, Code: code, Error: message}
}

func succeededStep(index int) actionResult {
	return actionResult{Index: index, Success: true, Result: map[string]any{"ok": true}}
}

const firstBatchFailure = "ref e94 not found - take a /snapshot first"

// The card's repro, as a differential: the same shared writer, one run in which
// every step failed and one in which every step succeeded. Every channel is
// asserted in BOTH directions from one table, so a channel added later inherits
// the rule by being added to the map rather than by being remembered.
func TestAFullyFailedMultiStepRunIsRecordedInEveryChannel(t *testing.T) {
	for _, surface := range []struct {
		name  string
		path  string
		extra map[string]any
	}{
		{"batch", "/tabs/tab1/actions", nil},
		{"macro", "/macro", map[string]any{"kind": "macro"}},
	} {
		t.Run(surface.name, func(t *testing.T) {
			failedRun := runMultiStep(t, surface.path, []actionResult{
				failedStep(0, "ref_not_found", firstBatchFailure),
				failedStep(1, "ref_not_found", "ref e95 not found - take a /snapshot first"),
				failedStep(2, "ref_not_found", "ref e96 not found - take a /snapshot first"),
			}, surface.extra)

			for name, envelope := range failedRun.channels {
				if !strings.Contains(envelope, "3 of 3 steps failed") {
					t.Errorf("%s does not say how many steps failed:\n%s", name, envelope)
				}
				if !strings.Contains(envelope, firstBatchFailure) {
					t.Errorf("%s does not carry the first failure's message:\n%s", name, envelope)
				}
				if !strings.Contains(envelope, "ref_not_found") {
					t.Errorf("%s does not carry the first failure's code:\n%s", name, envelope)
				}
			}
			if failedRun.failedDelta != 1 {
				t.Errorf("requestsFailed moved by %d, want 1 — an operator watching this number saw a clean record", failedRun.failedDelta)
			}
			if !strings.Contains(failedRun.channels["the server log"], "level=WARN") {
				t.Errorf("the run logged below WARN, so `grep level=WARN` finds nothing:\n%s", failedRun.channels["the server log"])
			}
			if len(failedRun.failures) == 0 {
				t.Fatal("failures.recent has no entry for a run in which every step failed")
			}
			last := failedRun.failures[len(failedRun.failures)-1]
			if got, _ := last["path"].(string); got != surface.path {
				t.Errorf("failures.recent path = %q, want %q", got, surface.path)
			}
			if got, _ := last["status"].(float64); got != 200 {
				if code, ok := last["status"].(int); !ok || code != 200 {
					t.Errorf("failures.recent status = %v, want the 200 the endpoint actually answered", last["status"])
				}
			}
			assertStepCounts(t, failedRun.activity, 3, 0, 3)

			// The other half of the differential. Without it, a change that
			// recorded every run as failed would satisfy every assertion above.
			okRun := runMultiStep(t, surface.path, []actionResult{
				succeededStep(0), succeededStep(1), succeededStep(2),
			}, surface.extra)

			for name, envelope := range okRun.channels {
				for _, forbidden := range []string{"steps failed", firstBatchFailure, "ref_not_found"} {
					if strings.Contains(envelope, forbidden) {
						t.Errorf("%s reports %q for a run in which every step succeeded:\n%s", name, forbidden, envelope)
					}
				}
			}
			if okRun.failedDelta != 0 {
				t.Errorf("a successful run moved requestsFailed by %d", okRun.failedDelta)
			}
			if len(okRun.failures) != 0 {
				t.Errorf("a successful run left %d entries in failures.recent", len(okRun.failures))
			}
			if !strings.Contains(okRun.channels["the server log"], "level=INFO") {
				t.Errorf("a successful run did not log at INFO:\n%s", okRun.channels["the server log"])
			}
			assertStepCounts(t, okRun.activity, 3, 3, 0)
		})
	}
}

// A partially failed run is still a failure: the successes already happened and
// the body carries them, but an operator watching the failure record must see that
// something did not.
func TestAPartiallyFailedMultiStepRunIsRecordedToo(t *testing.T) {
	run := runMultiStep(t, "/tabs/tab1/actions", []actionResult{
		succeededStep(0),
		failedStep(1, "action_failed", "click failed: element detached"),
	}, nil)

	if run.failedDelta != 1 {
		t.Errorf("requestsFailed moved by %d, want 1", run.failedDelta)
	}
	if !strings.Contains(run.channels["failures.recent"], "1 of 2 steps failed") {
		t.Errorf("failures.recent does not carry the partial count:\n%s", run.channels["failures.recent"])
	}
	assertStepCounts(t, run.activity, 2, 1, 1)
}

func assertStepCounts(t *testing.T, event map[string]any, total, successful, failed int) {
	t.Helper()
	steps, ok := event["steps"].(map[string]any)
	if !ok {
		t.Fatalf("the activity record carries no step counts: %v", event)
	}
	for field, want := range map[string]int{"total": total, "successful": successful, "failed": failed} {
		if got, _ := steps[field].(float64); int(got) != want {
			t.Errorf("activity steps.%s = %v, want %d", field, steps[field], want)
		}
	}
}

// proxiedRun drives the FRONT DOOR's shape: an upstream instance answering exactly
// what the shared writer now produces, through the real proxy, under the real
// logging middleware. The differential above is the bridge shape — one process, no
// hop — which is why it could not see that the reason stopped at the boundary.
func proxiedRun(t *testing.T, status int, code, message string) recordedRun {
	t.Helper()

	logs := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	resetObservabilityForTests()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if code != "" {
			// The producer's own publisher, so the headers under test are the ones a
			// real instance stamps rather than a literal this test invented.
			httpx.RecordFailureReason(w, code, message)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(upstream.Close)

	target, err := url.Parse(upstream.URL + "/tabs/tab1/actions")
	if err != nil {
		t.Fatal(err)
	}
	chain := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.Forward(w, r, target, proxy.Options{})
	}))

	req := httptest.NewRequest(http.MethodPost, "/tabs/tab1/actions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	failed, _ := SnapshotMetrics()["requestsFailed"].(uint64)
	failures := allRecentFailureEvents(t)
	encoded, err := json.Marshal(failures)
	if err != nil {
		t.Fatal(err)
	}
	return recordedRun{
		channels:    map[string]string{"the server log": logs.String(), "failures.recent": string(encoded)},
		failures:    failures,
		failedDelta: failed,
	}
}

// The hop the card's fix stopped at. GET /metrics on the front door serves the
// front door's OWN counters by design, so an operator running `pinchtab server`
// watches these numbers — and for a batch in which every step failed they did not
// move, because the reason travelled beside a 200 and the proxy dropped it on the
// status.
//
// The healthy row is the load-bearing half: removing the status gate is exactly the
// change that could start counting ordinary proxied traffic as failures, and the
// header's presence is what must be doing the work instead.
func TestTheFrontDoorRecordsAProxiedFailureReasonBesideA200(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		code       string
		message    string
		wantFailed uint64
		wantLevel  string
	}{
		{"a batch: 200 carrying a reason", http.StatusOK, "ref_not_found", "3 of 3 steps failed: " + firstBatchFailure, 1, "level=WARN"},
		{"a single action: 4xx carrying a reason", http.StatusNotFound, "ref_not_found", firstBatchFailure, 1, "level=WARN"},
		{"ordinary traffic: 200 with no reason", http.StatusOK, "", "", 0, "level=INFO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := proxiedRun(t, tc.status, tc.code, tc.message)

			if run.failedDelta != tc.wantFailed {
				t.Errorf("the front door's requestsFailed moved by %d, want %d — this is the counter an operator curls at /metrics",
					run.failedDelta, tc.wantFailed)
			}
			if len(run.failures) != int(tc.wantFailed) {
				t.Errorf("failures.recent holds %d entries, want %d:\n%s", len(run.failures), tc.wantFailed, run.channels["failures.recent"])
			}
			if !strings.Contains(run.channels["the server log"], tc.wantLevel) {
				t.Errorf("the front door logged at the wrong level, want %s:\n%s", tc.wantLevel, run.channels["the server log"])
			}
			if tc.wantFailed > 0 && !strings.Contains(run.channels["failures.recent"], tc.message) {
				t.Errorf("failures.recent does not carry the reason the instance published:\n%s", run.channels["failures.recent"])
			}
		})
	}
}
