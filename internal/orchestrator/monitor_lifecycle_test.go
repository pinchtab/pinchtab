package orchestrator

import (
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// Shutdown must not leave a startup monitor running. A monitor writes instance
// state for up to instanceStartupTimeout after launch, so one that outlives its
// orchestrator keeps touching memory the owner has finished with — and, under
// test, package vars the test has already restored.
//
// It must also not wait that timeout out. With a runner whose instance never
// becomes healthy, the probe loop would otherwise poll for the full 45s; the
// point of shutdownCh is that the loop notices and returns.
func TestShutdownStopsAndJoinsStartupMonitors(t *testing.T) {

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.hooks.portAvailable = func(int) bool { return true }
	o.hooks.processAlive = func(pid int) bool { return pid > 0 }
	if _, err := o.LaunchWithOptions("monitor-lifecycle", "9071", true, LaunchOptions{}); err != nil {
		t.Fatalf("LaunchWithOptions: %v", err)
	}

	// The instance is still starting, so the monitor is inside its probe loop.
	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		o.Shutdown()
		done <- time.Since(start)
	}()

	select {
	case elapsed := <-done:
		// The bound is half instanceStartupTimeout, which is the property under
		// test: Shutdown must not wait the startup probe out. It is not a
		// performance assertion — Stop already spends up to
		// gracefulProcessStopTimeout per instance waiting for the process to
		// exit, and this same test measures ~10s against an unmodified
		// orchestrator too. Joining the monitors adds nothing to that.
		if elapsed > instanceStartupTimeout/2 {
			t.Fatalf("Shutdown took %s; it waited out the startup probe instead of interrupting it", elapsed)
		}
	case <-time.After(instanceStartupTimeout):
		t.Fatal("Shutdown did not return; it is waiting out the full startup timeout")
	}

	// Shutdown returning means monitors.Wait() returned, so every monitor has
	// finished. Calling it again must stay safe — closing shutdownCh twice
	// would panic without the sync.Once.
	o.Shutdown()
}

// Shutdown is reachable more than once (an explicit call plus a signal handler),
// and on an Orchestrator whose shutdown channel was never made. Neither may panic
// on the nil channel.
func TestSignalShutdownIsIdempotentAndNilSafe(t *testing.T) {
	bare := withInstanceManager(&Orchestrator{})
	if bare.shuttingDown() {
		t.Fatal("a fresh orchestrator reports itself shutting down")
	}
	bare.signalShutdown()
	bare.signalShutdown()

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	if o.shuttingDown() {
		t.Fatal("a fresh orchestrator reports itself shutting down")
	}
	o.signalShutdown()
	if !o.shuttingDown() {
		t.Fatal("shuttingDown() is false after signalShutdown()")
	}
	o.signalShutdown()
}

// startMonitor is what registers a monitor with the WaitGroup Shutdown waits on.
// A bare `go o.monitor(...)` compiles, runs, and silently reintroduces the leak,
// so pin the call shape rather than trusting review to catch it.
func TestEveryMonitorGoesThroughStartMonitor(t *testing.T) {
	bareGoMonitor := regexp.MustCompile(`go\s+o\.monitor\(`)

	var offenders []string
	for name, body := range srccensus.Load(t, ".", 20).Sources() {
		for line := range strings.SplitSeq(body, "\n") {
			if bareGoMonitor.MatchString(line) {
				offenders = append(offenders, name)
				break
			}
		}
	}

	if len(offenders) > 0 {
		slices.Sort(offenders)
		t.Fatalf("monitor started outside startMonitor in %s; use o.startMonitor(inst) so Shutdown can wait for it",
			strings.Join(offenders, ", "))
	}
}

// Shutdown must also join the detached failed-attempt teardowns. A teardown
// kills a browser process and deletes its profile for up to its own escalation
// bound after Shutdown is called, so one that outlives the orchestrator is the
// same leak the monitors had — and its field comment already promised that
// shutdown paths wait for it. That tearDownFailedAttempt registers with the
// group is pinned by TestTearDownFailedAttempt_DetachesBeforeAsyncStop; this pins
// the other half.
func TestShutdownJoinsDetachedFailedAttemptTeardowns(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	var finished atomic.Bool
	o.detachedStops.Add(1)
	go func() {
		defer o.detachedStops.Done()
		time.Sleep(300 * time.Millisecond)
		finished.Store(true)
	}()

	o.Shutdown()

	if !finished.Load() {
		t.Fatal("Shutdown returned while a detached failed-attempt teardown was still running")
	}
}
