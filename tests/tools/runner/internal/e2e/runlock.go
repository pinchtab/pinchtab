package e2e

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type runLock struct {
	path string
}

type runLockHeldError struct {
	path string
	pid  int
}

func (e *runLockHeldError) Error() string {
	return fmt.Sprintf("e2e already running: lock %s is held by PID %d", e.path, e.pid)
}

// The compose stacks bind fixed host ports and share one image tag, so the
// lock is host-wide rather than per checkout.
func defaultRunLockPath() string {
	return filepath.Join(os.TempDir(), "pinchtab-e2e.lock")
}

func acquireRunLock(path string) (*runLock, error) {
	for range 2 {
		err := publishRunLock(path)
		if err == nil {
			return &runLock{path: path}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("e2e: take run lock %s: %w", path, err)
		}
		if pid, ok := runLockHolder(path); ok && processAlive(pid) {
			return nil, &runLockHeldError{path: path, pid: pid}
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("e2e: clear stale run lock %s: %w", path, err)
		}
	}
	return nil, fmt.Errorf("e2e: take run lock %s: lost the race for a stale lock twice", path)
}

// publishRunLock links a fully written PID file into place, so a concurrent
// reader never sees a lock without its holder.
func publishRunLock(path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, writeErr := fmt.Fprintf(tmp, "%d\n", os.Getpid())
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	return os.Link(tmp.Name(), path)
}

func runLockHolder(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid, err == nil && pid > 0
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	defer func() { _ = proc.Release() }()
	if runtime.GOOS == "windows" {
		return true
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func underRunLock(dryRun bool, path string, stderr io.Writer, run func() int) int {
	if dryRun {
		return run()
	}
	lock, err := acquireRunLock(path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer lock.release()
	return run()
}

func (l *runLock) release() {
	if pid, ok := runLockHolder(l.path); ok && pid == os.Getpid() {
		_ = os.Remove(l.path)
	}
}
