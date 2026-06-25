//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess        = modkernel32.NewProc("OpenProcess")
	procGetExitCodeProcess = modkernel32.NewProc("GetExitCodeProcess")
)

const (
	processQueryLimitedInfo = 0x1000
	stillActive             = 259 // STILL_ACTIVE / STATUS_PENDING — process has not exited
)

// processAlive confirms a process is still running. os.FindProcess/Signal(0) is
// useless on Windows (FindProcess always succeeds and Go has no Unix signals), so
// open a query handle (lookup) and check the exit code (confirm running): alive
// iff the exit code is STILL_ACTIVE. Mirrors
// internal/orchestrator/process_windows.go.
func processAlive(pid int) bool {
	handle, _, _ := procOpenProcess.Call(uintptr(processQueryLimitedInfo), 0, uintptr(pid))
	if handle == 0 {
		// Cannot open the process — it doesn't exist (or access denied). Assume
		// dead: for managing our own spawned server PID this won't arise, and
		// "dead when we can't confirm alive" is the safe default for the
		// stop/cleanup loops (avoids spinning on a PID we can't probe).
		return false
	}
	defer func() { _ = syscall.CloseHandle(syscall.Handle(handle)) }()

	var exitCode uint32
	ret, _, _ := procGetExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	if ret == 0 {
		return false // API call failed
	}
	return exitCode == stillActive
}

func stopProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func forceKillProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
