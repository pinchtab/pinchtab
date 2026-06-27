//go:build !windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

func processAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// A zombie (defunct) process still answers kill(pid, 0) because its PID
	// lingers in the table until the parent reaps it, but it is not running. In
	// containers whose PID 1 doesn't reap (e.g. a bare `sleep infinity`), a
	// SIGKILLed server stays a zombie — which would otherwise make stop/restart
	// report a false "did not exit" failure and refuse to start over it. Treat a
	// zombie as dead. (The official image's dumb-init reaps, so this only bites
	// minimal/init-less containers, but the check is correct everywhere.)
	return !processIsZombie(pid)
}

// processIsZombie reports whether pid is in the Linux zombie ("Z") state. It
// reads /proc/<pid>/stat; where /proc is absent (e.g. macOS, where launchd
// reaps and this case doesn't arise) it returns false and processAlive keeps
// its plain kill(0) semantics.
func processIsZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	// Format: "<pid> (<comm>) <state> ...". comm may contain spaces and ')', so
	// scan past the final ')' before reading the single-char state field.
	s := string(data)
	close := strings.LastIndexByte(s, ')')
	if close < 0 || close+2 >= len(s) {
		return false
	}
	return s[close+2] == 'Z'
}

func stopProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func forceKillProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
