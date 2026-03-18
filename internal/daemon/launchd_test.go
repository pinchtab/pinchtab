package daemon

import (
	"errors"
	"os"
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
	if !strings.Contains(content, "<string>/tmp/pinchtab/config.json</string>") {
		t.Fatalf("expected config path in plist: %s", content)
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
