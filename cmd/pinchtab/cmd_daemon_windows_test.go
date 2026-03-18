//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTaskManagerInstallCreatesScriptAndTask(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{
		outputs: map[string]string{},
	}
	manager := &windowsTaskManager{
		env: daemonEnvironment{
			homeDir:  root,
			osName:   "windows",
			execPath: `C:\Program Files\PinchTab\pinchtab.exe`,
		},
		runner: runner,
	}

	message, err := manager.Install(`C:\Program Files\PinchTab\pinchtab.exe`, `C:\Users\test\.config\pinchtab\config.json`)
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !strings.Contains(message, "PinchTab") {
		t.Fatalf("install message = %q, expected to mention PinchTab", message)
	}

	// Verify script was written
	scriptPath := filepath.Join(root, ".config", "pinchtab", "pinchtab-daemon.cmd")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("reading daemon script: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "pinchtab.exe") {
		t.Fatalf("script missing executable path: %s", content)
	}
	if !strings.Contains(content, "PINCHTAB_CONFIG") {
		t.Fatalf("script missing config env: %s", content)
	}

	// Verify schtasks calls were made
	foundCreate := false
	for _, call := range runner.calls {
		if strings.Contains(call, "/Create") && strings.Contains(call, "PinchTab") {
			foundCreate = true
		}
	}
	if !foundCreate {
		t.Fatalf("expected schtasks /Create call, got: %v", runner.calls)
	}
}

func TestWindowsTaskManagerUninstall(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{}
	manager := &windowsTaskManager{
		env: daemonEnvironment{
			homeDir: root,
			osName:  "windows",
		},
		runner: runner,
	}

	// Create the script file so uninstall can remove it
	scriptDir := filepath.Join(root, ".config", "pinchtab")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(scriptDir, "pinchtab-daemon.cmd")
	if err := os.WriteFile(scriptPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	message, err := manager.Uninstall()
	if err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if !strings.Contains(message, "uninstalled") {
		t.Fatalf("uninstall message = %q", message)
	}

	// Script should be removed
	if _, err := os.Stat(scriptPath); err == nil {
		t.Fatal("expected script to be removed")
	}
}

func TestWindowsTaskManagerServicePath(t *testing.T) {
	manager := &windowsTaskManager{
		env: daemonEnvironment{
			homeDir: `C:\Users\test`,
		},
	}
	got := manager.ServicePath()
	want := filepath.Join(`C:\Users\test`, ".config", "pinchtab", "pinchtab-daemon.cmd")
	if got != want {
		t.Fatalf("ServicePath() = %q, want %q", got, want)
	}
}
