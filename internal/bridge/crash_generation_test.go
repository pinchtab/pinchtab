package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBrowserContextGenerationIsStablePerContext(t *testing.T) {
	ResetCrashMonitoringForTests()
	t.Cleanup(ResetCrashMonitoringForTests)

	first := context.Background()
	second := t.Context()

	genFirst := BrowserContextGeneration(first)
	if again := BrowserContextGeneration(first); again != genFirst {
		t.Fatalf("same context produced generations %d and %d", genFirst, again)
	}
	genSecond := BrowserContextGeneration(second)
	if genSecond == genFirst {
		t.Fatalf("a new browser context reused generation %d", genSecond)
	}
	// A bridge with no browser context yet must not claim a generation, or a
	// crash would attach itself to "no browser".
	var missing context.Context
	if BrowserContextGeneration(missing) != 0 {
		t.Fatal("a nil context must not claim a generation")
	}
}

// A crash belongs to the browser that was live when it happened. After that
// browser is replaced, requests served on the new context must not be told it
// crashed — otherwise one transient fault reports forever.
func TestCrashForBrowserContextIgnoresEarlierGenerations(t *testing.T) {
	ResetCrashMonitoringForTests()
	t.Cleanup(ResetCrashMonitoringForTests)

	crashed, cancelCrashed := context.WithCancel(t.Context())
	defer cancelCrashed()
	RecordCrashForContextTests(crashed, CrashEvent{Reason: "inspector.targetCrashed"})

	got, ok := CrashForBrowserContext(crashed)
	if !ok || got.Reason != "inspector.targetCrashed" {
		t.Fatalf("crash on the live context = %+v (ok=%v), want the recorded crash", got, ok)
	}

	restarted, cancelRestarted := context.WithCancel(t.Context())
	defer cancelRestarted()
	if got, ok := CrashForBrowserContext(restarted); ok {
		t.Fatalf("a replaced browser still reports the old crash: %+v", got)
	}
}

// The crash buffer is process-global and shared across every instance, so a
// crash on one must not answer for a request served on another.
func TestCrashForBrowserContextIsPerInstance(t *testing.T) {
	ResetCrashMonitoringForTests()
	t.Cleanup(ResetCrashMonitoringForTests)

	instanceA, cancelA := context.WithCancel(t.Context())
	defer cancelA()
	instanceB, cancelB := context.WithCancel(t.Context())
	defer cancelB()

	// Both contexts are seen before the crash, so this is not the "old
	// generation" case: instance B is live and simply did not crash.
	BrowserContextGeneration(instanceB)
	RecordCrashForContextTests(instanceA, CrashEvent{Reason: "killed"})

	if _, ok := CrashForBrowserContext(instanceB); ok {
		t.Fatal("a crash on another instance reported against a healthy one")
	}
	if _, ok := CrashForBrowserContext(instanceA); !ok {
		t.Fatal("the crashed instance stopped reporting its own crash")
	}
}

func TestCrashForBrowserContextReturnsTheMostRecentCrash(t *testing.T) {
	ResetCrashMonitoringForTests()
	t.Cleanup(ResetCrashMonitoringForTests)

	ctx := t.Context()
	RecordCrashForContextTests(ctx, CrashEvent{Reason: "inspector.targetCrashed", Time: time.Unix(1, 0)})
	RecordCrashForContextTests(ctx, CrashEvent{Reason: "unexpected context cancellation", Time: time.Unix(2, 0)})

	got, ok := CrashForBrowserContext(ctx)
	if !ok || got.Reason != "unexpected context cancellation" {
		t.Fatalf("crash = %+v (ok=%v), want the most recent one", got, ok)
	}
}

// The generation is bookkeeping for the liveness lookup; /health and /metrics
// serialize CrashSnapshot directly and must not gain a field.
func TestCrashSnapshotDoesNotExposeTheGeneration(t *testing.T) {
	ResetCrashMonitoringForTests()
	t.Cleanup(ResetCrashMonitoringForTests)

	ctx := t.Context()
	RecordCrashForContextTests(ctx, CrashEvent{Reason: "killed", TargetID: "T1"})

	encoded, err := json.Marshal(CrashSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(encoded); strings.Contains(got, "eneration") {
		t.Fatalf("crash snapshot exposes the generation: %s", got)
	}
	if got := string(encoded); !strings.Contains(got, `"reason":"killed"`) || !strings.Contains(got, `"targetId":"T1"`) {
		t.Fatalf("crash snapshot lost its existing fields: %s", got)
	}
}
