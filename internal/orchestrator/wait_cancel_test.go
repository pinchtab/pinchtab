package orchestrator

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

const cancelSlack = 100 * time.Millisecond

func orchestratorAnsweringHealth(t *testing.T, status int) *Orchestrator {
	t.Helper()
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.client = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader("{}")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	return o
}

func waitInstance() *InstanceInternal {
	return &InstanceInternal{
		Instance: bridge.Instance{ID: "inst_wait0001", Port: "9999", Status: "starting"},
		URL:      "http://127.0.0.1:9999",
	}
}

func assertStopsWithinOneInterval(t *testing.T, interval time.Duration, wait func(context.Context)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan time.Time, 1)
	go func() {
		wait(ctx)
		done <- time.Now()
	}()
	time.Sleep(interval / 2)
	cancelledAt := time.Now()
	cancel()
	select {
	case returnedAt := <-done:
		if lag := returnedAt.Sub(cancelledAt); lag > interval+cancelSlack {
			t.Fatalf("wait returned %v after cancellation, want within one %v interval", lag, interval)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait ignored context cancellation")
	}
}

func TestWaitForProcessExitStopsOnCancellation(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.hooks.processAlive = func(int) bool { return true }

	assertStopsWithinOneInterval(t, processExitPollInterval, func(ctx context.Context) {
		if o.waitForProcessExit(ctx, 4242, time.Minute) {
			t.Error("a still-running process was reported as exited")
		}
	})
}

func TestWaitForBridgeEndpointExitStopsOnCancellation(t *testing.T) {
	o := orchestratorAnsweringHealth(t, http.StatusOK)
	inst := waitInstance()

	assertStopsWithinOneInterval(t, bridgeEndpointExitPollInterval, func(ctx context.Context) {
		if o.waitForBridgeEndpointExit(ctx, inst, time.Minute) {
			t.Error("cancellation was mistaken for the endpoint going away")
		}
	})
}

func TestWaitForChildBridgeHealthyStopsOnCancellation(t *testing.T) {
	o := orchestratorAnsweringHealth(t, http.StatusServiceUnavailable)
	inst := waitInstance()

	assertStopsWithinOneInterval(t, childBridgeHealthPollInterval, func(ctx context.Context) {
		if err := o.waitForChildBridgeHealthy(ctx, inst, time.Minute); err == nil {
			t.Error("an unhealthy bridge was reported healthy")
		}
	})
	if inst.Status != "starting" {
		t.Errorf("status = %q, want starting after a cancelled wait", inst.Status)
	}
}

func TestShutdownCancelsTheAttachHealthWait(t *testing.T) {
	o := orchestratorAnsweringHealth(t, http.StatusServiceUnavailable)
	ctx, cancel := o.shutdownScopedContext()
	defer cancel()
	o.signalShutdown()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the shutdown-scoped context")
	}
}
