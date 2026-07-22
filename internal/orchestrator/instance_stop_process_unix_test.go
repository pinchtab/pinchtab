//go:build !windows

package orchestrator

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestStopReapsOwnedBridgeProcessGroup(t *testing.T) {
	originalRequest := shutdownRequestTimeout
	originalGrace := gracefulProcessStopTimeout
	originalTerm := termProcessStopTimeout
	originalKill := killProcessStopTimeout
	shutdownRequestTimeout = 50 * time.Millisecond
	gracefulProcessStopTimeout = 50 * time.Millisecond
	termProcessStopTimeout = 100 * time.Millisecond
	killProcessStopTimeout = time.Second
	t.Cleanup(func() {
		shutdownRequestTimeout = originalRequest
		gracefulProcessStopTimeout = originalGrace
		termProcessStopTimeout = originalTerm
		killProcessStopTimeout = originalKill
	})

	runner := &LocalRunner{}
	cmd, err := runner.Run(
		context.Background(), "/bin/sh",
		[]string{"-c", `trap '' TERM; while :; do sleep 1; done`},
		os.Environ(), io.Discard, io.Discard,
	)
	if err != nil {
		t.Fatalf("start helper bridge process: %v", err)
	}
	t.Cleanup(func() {
		_ = killProcessGroup(cmd.PID(), sigKILL)
		cmd.Cancel()
	})
	waited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waited)
	}()

	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.instances["owned"] = &InstanceInternal{
		Instance: bridge.Instance{
			ID: "owned", URL: "http://127.0.0.1:1", Status: "running",
			Attached: true, AttachType: "cdp-bridge",
		},
		URL: "http://127.0.0.1:1",
		cmd: cmd,
	}
	if err := o.Stop("owned"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("owned bridge process was not reaped")
	}
	if isProcessAlive(cmd.PID()) {
		t.Fatalf("owned bridge PID %d is still alive", cmd.PID())
	}
	if _, present := o.instances["owned"]; present {
		t.Fatal("stopped instance remains registered")
	}
}
