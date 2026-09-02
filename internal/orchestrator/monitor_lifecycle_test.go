package orchestrator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
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
// and on an Orchestrator built as a bare struct literal, which several tests in
// this package do. Neither may panic on the nil channel.
func TestSignalShutdownIsIdempotentAndNilSafe(t *testing.T) {
	bare := &Orchestrator{}
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

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("cannot read package dir, so this rule would check nothing: %v", err)
	}

	scanned := 0
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(".", name))
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		scanned++
		for _, line := range strings.Split(string(body), "\n") {
			// Comments describe the shape; only code reintroduces it.
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if bareGoMonitor.MatchString(line) {
				offenders = append(offenders, name)
				break
			}
		}
	}

	if scanned < 20 {
		t.Fatalf("scanned %d non-test sources, want at least 20; the walk matched almost nothing and this rule would pass vacuously", scanned)
	}
	if len(offenders) > 0 {
		t.Fatalf("monitor started outside startMonitor in %s; use o.startMonitor(inst) so Shutdown can wait for it",
			strings.Join(offenders, ", "))
	}
}
