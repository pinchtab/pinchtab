package main

import (
	"os"
	"testing"
)

// TestProcessAlive_SelfIsAlive runs on every OS: the test process itself must be
// reported alive. On Windows this exercises the OpenProcess + GetExitCodeProcess
// path returning STILL_ACTIVE; on unix the syscall.Kill(pid, 0) path.
func TestProcessAlive_SelfIsAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatalf("processAlive(self=%d) = false, want true", os.Getpid())
	}
}
