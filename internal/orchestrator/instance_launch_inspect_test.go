package orchestrator

import (
	"context"
	"io"
	"testing"
	"time"
)

type blockingInspectRunner struct {
	inspecting chan struct{}
	release    chan struct{}
}

func (b *blockingInspectRunner) Run(context.Context, string, []string, []string, io.Writer, io.Writer) (Cmd, error) {
	return &mockCmd{pid: 1234, isAlive: true}, nil
}

func (b *blockingInspectRunner) InspectPort(string) PortInspection {
	close(b.inspecting)
	<-b.release
	return PortInspection{Available: false, PID: 42, Command: "squatter"}
}

func TestSlowPortInspectionDoesNotHoldTheOrchestratorLock(t *testing.T) {
	stubPortAvailability(t, func(int) bool { return true })
	runner := &blockingInspectRunner{inspecting: make(chan struct{}), release: make(chan struct{})}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)

	launched := make(chan error, 1)
	go func() {
		_, err := o.LaunchWithOptions("slow-port", "", true, LaunchOptions{})
		launched <- err
	}()
	select {
	case <-runner.inspecting:
	case <-time.After(5 * time.Second):
		t.Fatal("launch never reached the port inspection")
	}

	listed := make(chan struct{})
	go func() {
		o.List()
		close(listed)
	}()
	select {
	case <-listed:
	case <-time.After(2 * time.Second):
		t.Fatal("List blocked while InspectPort was running")
	}

	close(runner.release)
	if err := <-launched; err == nil {
		t.Fatal("launch succeeded on a port InspectPort reported busy")
	}
}
