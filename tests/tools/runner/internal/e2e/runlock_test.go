package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func exitedProcessPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatalf("run throwaway child: %v", err)
	}
	return cmd.Process.Pid
}

func TestRunLockAcquireRefuseAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.lock")

	lock, err := acquireRunLock(path)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	_, err = acquireRunLock(path)
	var held *runLockHeldError
	if !errors.As(err, &held) {
		t.Fatalf("second acquire error = %v, want runLockHeldError", err)
	}
	want := fmt.Sprintf("e2e already running: lock %s is held by PID %d", path, os.Getpid())
	if err.Error() != want {
		t.Fatalf("refusal = %q, want %q", err.Error(), want)
	}

	lock.release()
	relock, err := acquireRunLock(path)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	relock.release()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file should be gone after release, stat err = %v", err)
	}
}

func TestRunLockRecoversStaleLock(t *testing.T) {
	for name, content := range map[string]string{
		"dead holder":     fmt.Sprintf("%d\n", exitedProcessPID(t)),
		"unreadable lock": "not-a-pid",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "e2e.lock")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			lock, err := acquireRunLock(path)
			if err != nil {
				t.Fatalf("stale lock blocked the run: %v", err)
			}
			defer lock.release()
			if pid, ok := runLockHolder(path); !ok || pid != os.Getpid() {
				t.Fatalf("lock holder = %d (ok=%v), want this process %d", pid, ok, os.Getpid())
			}
		})
	}
}

func TestRunLockReleaseLeavesAnotherHoldersLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.lock")
	lock, err := acquireRunLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock.release()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("release removed a lock this process no longer holds: %v", err)
	}
}

func TestRunRefusesWhileLockHeldAndStartsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.lock")
	lock, err := acquireRunLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()

	var stdout, stderr bytes.Buffer
	if code := runUnderLock(Args{Suite: "cli"}, path, &stdout, &stderr, runProvider); code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr.String(), "e2e already running: lock "+path) {
		t.Fatalf("stderr = %q, want the already-running refusal", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("a refused run must start nothing, stdout:\n%s", stdout.String())
	}
}

func TestDryRunIgnoresTheRunLock(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	path := filepath.Join(t.TempDir(), "e2e.lock")
	lock, err := acquireRunLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()

	var stdout, stderr bytes.Buffer
	if code := runUnderLock(Args{Suite: "cli", DryRun: true}, path, &stdout, &stderr, runProvider); code != 0 {
		t.Fatalf("dry run = %d under a held lock, stderr: %s", code, stderr.String())
	}
	if pid, ok := runLockHolder(path); !ok || pid != os.Getpid() {
		t.Fatalf("dry run disturbed the held lock: holder = %d (ok=%v)", pid, ok)
	}
}

func TestDryRunMatrixIgnoresTheRunLock(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	path := filepath.Join(t.TempDir(), "e2e.lock")
	lock, err := acquireRunLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()

	var stdout, stderr bytes.Buffer
	args := Args{Suite: "api", DryRun: true, Providers: []string{"chrome", "cloak"}}
	if code := runUnderLock(args, path, &stdout, &stderr, runProvider); code != 0 {
		t.Fatalf("dry-run matrix = %d under a held lock, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Browser matrix completed: all 2 browsers passed") {
		t.Fatalf("dry-run matrix did not run both browsers:\n%s", stdout.String())
	}
	if pid, ok := runLockHolder(path); !ok || pid != os.Getpid() {
		t.Fatalf("dry-run matrix disturbed the held lock: holder = %d (ok=%v)", pid, ok)
	}
}
