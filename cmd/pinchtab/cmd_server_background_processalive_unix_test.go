//go:build !windows

package main

import (
	"os/exec"
	"testing"
)

// TestProcessAlive_ReapedChildIsDead verifies the dead-PID contract on hosts the
// test can run: a child process that has fully exited and been reaped must be
// reported not-alive. (The Windows liveness path mirrors the proven
// internal/orchestrator/process_windows.go and is build-verified via cross-compile.)
func TestProcessAlive_ReapedChildIsDead(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start child process: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child wait: %v", err)
	}
	if processAlive(pid) {
		t.Fatalf("processAlive(reaped child=%d) = true, want false", pid)
	}
}
