//go:build !windows

package main

import "syscall"

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func stopProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func forceKillProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
