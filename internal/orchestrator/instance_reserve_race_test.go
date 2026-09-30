package orchestrator

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/ids"
)

type gatedRunner struct {
	entered chan struct{}
	release chan struct{}
}

func (g *gatedRunner) Run(context.Context, string, []string, []string, io.Writer, io.Writer) (Cmd, error) {
	g.entered <- struct{}{}
	<-g.release
	return &mockCmd{pid: 1234, isAlive: true}, nil
}

func (g *gatedRunner) InspectPort(string) PortInspection { return PortInspection{Available: true} }

func TestConcurrentSameProfileLaunchRefusesTheSecondWhileTheFirstSpawns(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	runner := &gatedRunner{entered: make(chan struct{}, 2), release: make(chan struct{})}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	t.Cleanup(func() {
		for _, inst := range o.List() {
			_ = o.Stop(inst.ID)
		}
	})

	firstErr := make(chan error, 1)
	go func() {
		_, err := o.LaunchWithOptions("dup-profile", "", true, LaunchOptions{})
		firstErr <- err
	}()
	select {
	case <-runner.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first launch never reached the spawn")
	}

	secondErr := make(chan error, 1)
	go func() {
		_, err := o.LaunchWithOptions("dup-profile", "", true, LaunchOptions{})
		secondErr <- err
	}()
	select {
	case err := <-secondErr:
		if err == nil || !strings.Contains(err.Error(), "already has an active instance") {
			t.Fatalf("second launch err = %v, want an already-active refusal", err)
		}
	case <-runner.entered:
		close(runner.release)
		t.Fatal("second launch reached the spawn while the first was still starting")
	case <-time.After(5 * time.Second):
		t.Fatal("second launch neither refused nor spawned")
	}

	close(runner.release)
	if err := <-firstErr; err != nil {
		t.Fatalf("first launch: %v", err)
	}
	if n := countInstancesNamed(o, "dup-profile"); n != 1 {
		t.Fatalf("instances named dup-profile = %d, want 1", n)
	}
}

func TestStopDuringLaunchSpawnIsRefusedAndLaunchPublishes(t *testing.T) {
	old := processAliveFunc
	processAliveFunc = func(pid int) bool { return pid > 0 }
	t.Cleanup(func() { processAliveFunc = old })
	stubPortAvailability(t, func(int) bool { return true })

	runner := &gatedRunner{entered: make(chan struct{}, 1), release: make(chan struct{})}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	launched := make(chan error, 1)
	go func() {
		_, err := o.LaunchWithOptions("stop-while-starting", "", true, LaunchOptions{})
		launched <- err
	}()
	select {
	case <-runner.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("launch never reached the spawn")
	}

	id := instanceIDNamed(o, "stop-while-starting")
	stopErr := o.Stop(id)
	close(runner.release)
	if err := <-launched; err != nil {
		t.Fatalf("launch: %v", err)
	}
	if stopErr == nil || !strings.Contains(stopErr.Error(), "still starting") {
		t.Fatalf("Stop during spawn err = %v, want a still-starting refusal", stopErr)
	}
	o.mu.RLock()
	inst, ok := o.instances[id]
	running := ok && inst.cmd != nil
	o.mu.RUnlock()
	if !running {
		t.Fatalf("launched instance missing from the registry after the refused stop")
	}
	t.Cleanup(func() { o.ForceShutdown() })
}

func TestFailedRelaunchKeepsThePreviousErrorRecord(t *testing.T) {
	stubPortAvailability(t, func(int) bool { return true })
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true, runErr: errors.New("exec format error")})

	previousID := ids.InstanceID(ids.ProfileID("relaunch"), "relaunch")
	previous := &InstanceInternal{Instance: bridge.Instance{ID: previousID, ProfileName: "relaunch", Status: "error", Error: "chrome crashed"}}
	o.instances[previousID] = previous

	if _, err := o.LaunchWithOptions("relaunch", "", true, LaunchOptions{}); err == nil {
		t.Fatal("relaunch succeeded, want the spawn failure")
	}
	if len(o.instances) != 1 || o.instances[previousID] != previous || previous.Instance.Error != "chrome crashed" {
		t.Fatalf("registry after failed relaunch = %+v, want only the previous error record", o.instances)
	}
}

func TestConcurrentSameNameExternalAttachLeavesOneInstance(t *testing.T) {
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})

	const callers = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			<-start
			_, _, err := o.attachExternalInstance("dup-attach", bridge.Instance{URL: "http://127.0.0.1:9222"}, "")
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded := 0
	for err := range errs {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful attaches = %d, want 1", succeeded)
	}
	if n := countInstancesNamed(o, "dup-attach"); n != 1 {
		t.Fatalf("instances named dup-attach = %d, want 1", n)
	}
}

func countInstancesNamed(o *Orchestrator, name string) int {
	n := 0
	for _, inst := range o.List() {
		if inst.ProfileName == name {
			n++
		}
	}
	return n
}

func instanceIDNamed(o *Orchestrator, name string) string {
	for _, inst := range o.List() {
		if inst.ProfileName == name {
			return inst.ID
		}
	}
	return ""
}
