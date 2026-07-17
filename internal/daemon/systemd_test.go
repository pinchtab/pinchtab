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
	stdoutLogPath := filepath.Join(root, ".pinchtab", "logs", "daemon.out.log")
	stderrLogPath := filepath.Join(root, ".pinchtab", "logs", "daemon.err.log")
	if !strings.Contains(content, "StandardOutput=append:"+stdoutLogPath) {
		t.Fatalf("expected stdout log path in unit content: %s", content)
	}
	if !strings.Contains(content, "StandardError=append:"+stderrLogPath) {
		t.Fatalf("expected stderr log path in unit content: %s", content)
	}
	if info, err := os.Stat(filepath.Join(root, ".pinchtab", "logs")); err != nil {
		t.Fatalf("expected log directory to exist: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected log directory, got file")
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

func TestSystemdUserManagerLogsFallsBackToJournalctl(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{
		outputs: map[string]string{
			"journalctl --user -u pinchtab.service -n 15 --no-pager": "journalctl output",
		},
	}
	manager := &systemdUserManager{
		env:    environment{homeDir: root, osName: "linux"},
		runner: runner,
	}

	output, err := manager.Logs(15)
	if err != nil {
		t.Fatalf("Logs returned error: %v", err)
	}
	if output != "journalctl output" {
		t.Fatalf("unexpected logs output: %q", output)
	}
	expected := "journalctl --user -u pinchtab.service -n 15 --no-pager"
	if len(runner.calls) != 1 || runner.calls[0] != expected {
		t.Fatalf("journalctl call = %v, want %q", runner.calls, expected)
	}
}

func TestSystemdUserManagerRestartUsesExactlyOneManagerCommand(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{}
	manager := &systemdUserManager{
		env:    environment{homeDir: root, osName: "linux"},
		runner: runner,
	}

	if _, err := manager.Restart(); err != nil {
		t.Fatalf("Restart returned error: %v", err)
	}
	want := []string{"systemctl --user restart pinchtab.service"}
	if strings.Join(runner.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("restart calls = %v, want %v", runner.calls, want)
	}
}
