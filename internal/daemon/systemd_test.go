package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUserManagerInstallWritesUnitAndEnablesService(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{}
	manager := &systemdUserManager{
		env: environment{
			homeDir:       root,
			osName:        "linux",
			execPath:      "/usr/local/bin/pinchtab",
			xdgConfigHome: filepath.Join(root, ".config"),
		},
		runner: runner,
	}

	message, err := manager.Install("/tmp/pinchtab/config.json")
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !strings.Contains(message, manager.ServicePath()) {
		t.Fatalf("install message = %q, want path %q", message, manager.ServicePath())
	}

	data, err := os.ReadFile(manager.ServicePath())
	if err != nil {
		t.Fatalf("reading service file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `ExecStart="/usr/local/bin/pinchtab" server`) {
		t.Fatalf("unexpected unit content: %s", content)
	}
	if !strings.Contains(content, `Environment="PINCHTAB_CONFIG=/tmp/pinchtab/config.json"`) {
		t.Fatalf("expected config env in unit content: %s", content)
	}

	expectedCalls := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now pinchtab.service",
	}
	if strings.Join(runner.calls, "\n") != strings.Join(expectedCalls, "\n") {
		t.Fatalf("systemd calls = %v, want %v", runner.calls, expectedCalls)
	}
}

func TestSystemdUserManagerPreflightRequiresUserSession(t *testing.T) {
	runner := &fakeCommandRunner{
		errors: map[string]error{
			"systemctl --user show-environment": errors.New("exit status 1"),
		},
	}
	manager := &systemdUserManager{
		env:    environment{osName: "linux"},
		runner: runner,
	}

	err := manager.Preflight()
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if !strings.Contains(err.Error(), "working user systemd session") {
		t.Fatalf("unexpected error: %v", err)
	}
}
