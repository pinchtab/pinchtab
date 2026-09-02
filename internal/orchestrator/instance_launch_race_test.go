package orchestrator

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// LaunchWithOptions starts o.monitor on the instance it just registered, and
// monitor -> applyStartupOutcome writes Status, URL and Error on that struct.
// Returning &inst.Instance handed the caller a live pointer into it, and
// POST /instances/start serializes exactly that pointer into the response with
// no lock held (handlers_instances.go, httpx.JSON(w, 201, inst)).
//
// The detector reports it as applyStartupOutcome against
// bridge.Instance.MarshalJSON. It is timing-dependent enough that a plain
// package run only surfaces it under load, so this reproduces the shape
// directly: marshal the returned value while the monitor is still writing.
//
// Fails under -race before the snapshot in LaunchWithOptions; passes after.
func TestLaunchReturnsAValueTheMonitorCannotWrite(t *testing.T) {
	if !raceDetectorEnabled {
		t.Skip("verifies nothing without the race detector; run with -race")
	}

	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	t.Cleanup(func() {
		for _, inst := range o.List() {
			_ = o.Stop(inst.ID)
		}
	})

	inst, err := o.LaunchWithOptions("race-probe", "9061", true, LaunchOptions{})
	if err != nil {
		t.Fatalf("LaunchWithOptions: %v", err)
	}

	// The response path: serialize what the caller was handed, continuously,
	// across the window in which the monitor writes. probeStartupHealth sleeps
	// instanceHealthPollInterval before its first probe, so the earliest write
	// lands ~500ms in; marshalling for a few multiples of that spans it without
	// depending on the exact scheduling.
	deadline := time.Now().Add(4 * instanceHealthPollInterval)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			if _, err := json.Marshal(inst); err != nil {
				t.Errorf("Marshal: %v", err)
				return
			}
		}
	}()
	wg.Wait()
}
