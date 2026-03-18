//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
)

const pinchtabTaskName = "PinchTab"

type windowsTaskManager struct {
	env    daemonEnvironment
	runner commandRunner
}

func newPlatformDaemonManager(env daemonEnvironment, runner commandRunner) (daemonManager, error) {
	return &windowsTaskManager{env: env, runner: runner}, nil
}

func managerEnvironmentPlatform(manager daemonManager) daemonEnvironment {
	if m, ok := manager.(*windowsTaskManager); ok {
		return m.env
	}
	return daemonEnvironment{}
}

func (m *windowsTaskManager) Preflight() error {
	// Verify schtasks.exe is available
	if _, err := runCommand(m.runner, "schtasks.exe", "/Query", "/TN", "\\"); err != nil {
		return fmt.Errorf("windows daemon install requires schtasks.exe to be available: %w", err)
	}
	return nil
}

func (m *windowsTaskManager) Install(execPath, configPath string) (string, error) {
	// Create a wrapper script that sets the config env and runs the server
	scriptDir := filepath.Join(m.env.homeDir, ".config", "pinchtab")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "pinchtab-daemon.cmd")
	script := fmt.Sprintf("@echo off\r\nset \"PINCHTAB_CONFIG=%s\"\r\n\"%s\" server\r\n", configPath, execPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return "", fmt.Errorf("write daemon script: %w", err)
	}

	// Remove existing task if present (ignore errors)
	_, _ = runCommand(m.runner, "schtasks.exe", "/Delete", "/TN", pinchtabTaskName, "/F")

	// Create a scheduled task that runs at logon
	_, err := runCommand(m.runner, "schtasks.exe",
		"/Create",
		"/TN", pinchtabTaskName,
		"/TR", scriptPath,
		"/SC", "ONLOGON",
		"/RL", "LIMITED",
		"/F",
	)
	if err != nil {
		return "", fmt.Errorf("create scheduled task: %w", err)
	}

	// Start the task immediately
	_, _ = runCommand(m.runner, "schtasks.exe", "/Run", "/TN", pinchtabTaskName)

	return fmt.Sprintf("Installed Windows scheduled task '%s' (runs at logon)", pinchtabTaskName), nil
}

func (m *windowsTaskManager) ServicePath() string {
	return filepath.Join(m.env.homeDir, ".config", "pinchtab", "pinchtab-daemon.cmd")
}

func (m *windowsTaskManager) Start() (string, error) {
	if _, err := runCommand(m.runner, "schtasks.exe", "/Run", "/TN", pinchtabTaskName); err != nil {
		return "", fmt.Errorf("start task: %w", err)
	}
	return "Pinchtab daemon started.", nil
}

func (m *windowsTaskManager) Restart() (string, error) {
	// Stop then start
	_, _ = m.Stop()
	return m.Start()
}

func (m *windowsTaskManager) Stop() (string, error) {
	if _, err := runCommand(m.runner, "schtasks.exe", "/End", "/TN", pinchtabTaskName); err != nil {
		// Also try taskkill as fallback
		_, _ = runCommand(m.runner, "taskkill.exe", "/IM", "pinchtab.exe", "/F")
	}
	return "Pinchtab daemon stopped.", nil
}

func (m *windowsTaskManager) Status() (string, error) {
	output, err := runCommand(m.runner, "schtasks.exe", "/Query", "/TN", pinchtabTaskName, "/V", "/FO", "LIST")
	if err != nil {
		return "", err
	}
	return output, nil
}

func (m *windowsTaskManager) Uninstall() (string, error) {
	var errs []error

	// Stop the task first
	_, _ = m.Stop()

	// Delete the scheduled task
	if _, err := runCommand(m.runner, "schtasks.exe", "/Delete", "/TN", pinchtabTaskName, "/F"); err != nil {
		errs = append(errs, fmt.Errorf("delete scheduled task: %w", err))
	}

	// Remove the wrapper script
	scriptPath := m.ServicePath()
	if err := os.Remove(scriptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("remove daemon script: %w", err))
	}

	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	return "Pinchtab daemon uninstalled.", nil
}

func (m *windowsTaskManager) Pid() (string, error) {
	// Use tasklist to find pinchtab.exe PID
	output, err := runCommand(m.runner, "tasklist.exe", "/FI", "IMAGENAME eq pinchtab.exe", "/FO", "CSV", "/NH")
	if err != nil {
		return "", err
	}
	// CSV format: "pinchtab.exe","1234","Console","1","12,345 K"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "pinchtab.exe") {
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				pid := strings.Trim(parts[1], "\" ")
				return pid, nil
			}
		}
	}
	return "", nil
}

func (m *windowsTaskManager) Logs(n int) (string, error) {
	// Check for log file in config directory
	logPath := filepath.Join(m.env.homeDir, ".config", "pinchtab", "pinchtab.log")
	if _, err := os.Stat(logPath); err != nil {
		return "No logs found. Windows daemon logs to stdout of the scheduled task.", nil
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}

func (m *windowsTaskManager) ManualInstructions() string {
	var b strings.Builder
	fmt.Fprintln(&b, cli.StyleStdout(cli.HeadingStyle, "Manual instructions (Windows/Task Scheduler):"))
	fmt.Fprintln(&b, cli.StyleStdout(cli.MutedStyle, "To install manually:"))
	fmt.Fprintln(&b, "  1. Open Task Scheduler (taskschd.msc)")
	fmt.Fprintln(&b, "  2. Create a new task named "+cli.StyleStdout(cli.ValueStyle, pinchtabTaskName))
	fmt.Fprintln(&b, "  3. Set trigger: At log on")
	fmt.Fprintln(&b, "  4. Set action: Start "+cli.StyleStdout(cli.CommandStyle, "pinchtab server"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, cli.StyleStdout(cli.MutedStyle, "To uninstall manually:"))
	fmt.Fprintln(&b, "  1. Run: "+cli.StyleStdout(cli.CommandStyle, "schtasks /Delete /TN PinchTab /F"))
	return b.String()
}
