package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchdManagerInstallWritesPlistAndBootstrapsAgent(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{}
	manager := &launchdManager{
		env: environment{
			homeDir:  root,
			osName:   "darwin",
			execPath: "/Applications/Pinchtab.app/Contents/MacOS/pinchtab",
			userID:   "501",
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
		t.Fatalf("reading launchd plist: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "<string>com.pinchtab.pinchtab</string>") {
		t.Fatalf("expected launchd label in plist: %s", content)
	}
	if !strings.Contains(content, "<string>/Applications/Pinchtab.app/Contents/MacOS/pinchtab</string>") {
		t.Fatalf("expected executable path in plist: %s", content)
	}
	if !strings.Contains(content, "<key>WorkingDirectory</key>") || !strings.Contains(content, "<string>"+root+"</string>") {
		t.Fatalf("expected working directory in plist: %s", content)
	}
	if !strings.Contains(content, "<key>HOME</key>") || !strings.Contains(content, "<string>"+root+"</string>") {
		t.Fatalf("expected HOME environment in plist: %s", content)
	}
	if !strings.Contains(content, "<string>/tmp/pinchtab/config.json</string>") {
		t.Fatalf("expected config path in plist: %s", content)
	}
	stdoutLogPath := filepath.Join(root, ".pinchtab", "logs", "daemon.out.log")
	stderrLogPath := filepath.Join(root, ".pinchtab", "logs", "daemon.err.log")
	if !strings.Contains(content, "<string>"+stdoutLogPath+"</string>") {
		t.Fatalf("expected stdout log path in plist: %s", content)
	}
	if !strings.Contains(content, "<string>"+stderrLogPath+"</string>") {
		t.Fatalf("expected stderr log path in plist: %s", content)
	}
	if info, err := os.Stat(filepath.Join(root, ".pinchtab", "logs")); err != nil {
		t.Fatalf("expected log directory to exist: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected log directory, got file")
	}

	expectedCalls := []string{
		"launchctl bootout gui/501 " + manager.ServicePath(),
		"launchctl bootstrap gui/501 " + manager.ServicePath(),
		"launchctl kickstart -k gui/501/com.pinchtab.pinchtab",
	}
	if strings.Join(runner.calls, "\n") != strings.Join(expectedCalls, "\n") {
		t.Fatalf("launchctl calls = %v, want %v", runner.calls, expectedCalls)
	}
}

func TestLaunchdManagerPreflightRequiresGUIDomain(t *testing.T) {
	runner := &fakeCommandRunner{
		errors: map[string]error{
			"launchctl print gui/501": errors.New("exit status 113"),
		},
	}
	manager := &launchdManager{
		env:    environment{osName: "darwin", userID: "501"},
		runner: runner,
	}

	err := manager.Preflight()
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if !strings.Contains(err.Error(), "active launchd GUI session") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLaunchdManagerLogsFallsBackToLegacyPath(t *testing.T) {
	root := t.TempDir()
	legacyLogPath := "/tmp/pinchtab.err.log"
	legacyContent, legacyErr := os.ReadFile(legacyLogPath)
	hadLegacy := legacyErr == nil
	if err := os.WriteFile(legacyLogPath, []byte("legacy launchd log\n"), 0644); err != nil {
		t.Fatalf("write legacy log: %v", err)
	}
	t.Cleanup(func() {
		if hadLegacy {
			_ = os.WriteFile(legacyLogPath, legacyContent, 0644)
		} else {
			_ = os.Remove(legacyLogPath)
		}
	})

	runner := &fakeCommandRunner{
		outputs: map[string]string{
			"tail -n 20 /tmp/pinchtab.err.log": "tail output",
		},
	}
	manager := &launchdManager{
		env:    environment{homeDir: root, osName: "darwin"},
		runner: runner,
	}

	output, err := manager.Logs(20)
	if err != nil {
		t.Fatalf("Logs returned error: %v", err)
	}
	if output != "tail output" {
		t.Fatalf("unexpected logs output: %q", output)
	}
	expected := "tail -n 20 /tmp/pinchtab.err.log"
	if len(runner.calls) != 1 || runner.calls[0] != expected {
		t.Fatalf("tail call = %v, want %q", runner.calls, expected)
	}
}

func TestLaunchdErrorClassification(t *testing.T) {
	bootstrapped := errors.New("Bootstrap failed: 5: Input/output error (service already bootstrapped)")
	absent := errors.New("Boot-out failed: 5: No such process")
	exit5 := errors.New("launchctl: exit status 5")
	notFound := errors.New("service not found")
	unrelated := errors.New("permission denied")

	// isLaunchdAlreadyBootstrapped: only the bootstrapped message; narrower than
	// the ignorable bundle (must NOT swallow the teardown "absent" cases).
	if !isLaunchdAlreadyBootstrapped(bootstrapped) {
		t.Error("isLaunchdAlreadyBootstrapped should match the bootstrapped message")
	}
	for _, err := range []error{absent, exit5, notFound, unrelated, nil} {
		if isLaunchdAlreadyBootstrapped(err) {
			t.Errorf("isLaunchdAlreadyBootstrapped should be false for %v", err)
		}
	}

	// isLaunchdIgnorableError: the full teardown bundle (incl. bootstrapped),
	// and nil is benign.
	for _, err := range []error{nil, bootstrapped, absent, exit5, notFound} {
		if !isLaunchdIgnorableError(err) {
			t.Errorf("isLaunchdIgnorableError should be true for %v", err)
		}
	}
	if isLaunchdIgnorableError(unrelated) {
		t.Error("isLaunchdIgnorableError should be false for an unrelated error")
	}
}
