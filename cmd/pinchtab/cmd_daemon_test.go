package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeCommandRunner struct {
	calls   []string
	outputs map[string]string
	errors  map[string]error
}

func (f *fakeCommandRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if out, ok := f.outputs[call]; ok {
		return []byte(out), f.errors[call]
	}
	return nil, f.errors[call]
}

func TestEnsureOnboardConfigCreatesDefaultConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")
	t.Setenv("PINCHTAB_BIND", "")

	gotPath, cfg, status, err := ensureDaemonConfig(false)
	if err != nil {
		t.Fatalf("ensureDaemonConfig returned error: %v", err)
	}
	if status != configCreated {
		t.Fatalf("status = %q, want %q", status, configCreated)
	}
	if gotPath != configPath {
		t.Fatalf("config path = %q, want %q", gotPath, configPath)
	}
	if cfg.Server.Bind != "127.0.0.1" {
		t.Fatalf("bind = %q, want 127.0.0.1", cfg.Server.Bind)
	}
	if strings.TrimSpace(cfg.Server.Token) == "" {
		t.Fatal("expected generated token to be set")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"bind": "127.0.0.1"`) {
		t.Fatalf("expected config file to include bind, got %s", content)
	}
	if !strings.Contains(content, `"token": "`) {
		t.Fatalf("expected config file to include token, got %s", content)
	}
}

func TestEnsureOnboardConfigRecoversExistingSecuritySettings(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	input := `{
  "server": {
    "bind": "0.0.0.0",
    "port": "9999",
    "token": ""
  },
  "browser": {
    "binary": "/custom/chrome"
  },
  "security": {
    "allowEvaluate": true
  }
}
`
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(input), 0644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	_, cfg, status, err := ensureDaemonConfig(false)
	if err != nil {
		t.Fatalf("ensureDaemonConfig returned error: %v", err)
	}
	// Only token recovery happens now — security settings are preserved for wizard
	if status != configRecovered {
		t.Fatalf("status = %q, want %q", status, configRecovered)
	}
	// Bind is preserved as-is (wizard handles security changes, not daemon config)
	if cfg.Server.Bind != "0.0.0.0" {
		t.Fatalf("bind = %q, want 0.0.0.0 (preserved)", cfg.Server.Bind)
	}
	if cfg.Server.Port != "9999" {
		t.Fatalf("port = %q, want 9999", cfg.Server.Port)
	}
	if cfg.Browser.ChromeBinary != "/custom/chrome" {
		t.Fatalf("chrome binary = %q, want /custom/chrome", cfg.Browser.ChromeBinary)
	}
	// Security settings preserved — not overwritten
	if !boolPtrValue(cfg.Security.AllowEvaluate) {
		t.Fatal("expected allowEvaluate to be preserved as true")
	}
	// Token should be generated (was empty)
	if strings.TrimSpace(cfg.Server.Token) == "" {
		t.Fatal("expected recovery to generate a token")
	}
}

func TestSystemdUserManagerInstallWritesUnitAndEnablesService(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{}
	manager := &systemdUserManager{
		env: daemonEnvironment{
			homeDir:       root,
			osName:        "linux",
			execPath:      "/usr/local/bin/pinchtab",
			xdgConfigHome: filepath.Join(root, ".config"),
		},
		runner: runner,
	}

	message, err := manager.Install("/usr/local/bin/pinchtab", "/tmp/pinchtab/config.json")
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

func TestLaunchdManagerInstallWritesPlistAndBootstrapsAgent(t *testing.T) {
	root := t.TempDir()
	runner := &fakeCommandRunner{}
	manager := &launchdManager{
		env: daemonEnvironment{
			homeDir:  root,
			osName:   "darwin",
			execPath: "/Applications/Pinchtab.app/Contents/MacOS/pinchtab",
			userID:   "501",
		},
		runner: runner,
	}

	message, err := manager.Install("/Applications/Pinchtab.app/Contents/MacOS/pinchtab", "/tmp/pinchtab/config.json")
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

func TestSystemdUserManagerPreflightRequiresUserSession(t *testing.T) {
	runner := &fakeCommandRunner{
		errors: map[string]error{
			"systemctl --user show-environment": errors.New("exit status 1"),
		},
	}
	manager := &systemdUserManager{
		env:    daemonEnvironment{osName: "linux"},
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

func TestLaunchdManagerPreflightRequiresGUIDomain(t *testing.T) {
	runner := &fakeCommandRunner{
		errors: map[string]error{
			"launchctl print gui/501": errors.New("exit status 113"),
		},
	}
	manager := &launchdManager{
		env:    daemonEnvironment{osName: "darwin", userID: "501"},
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

func TestNewDaemonManagerRejectsUnsupportedOS(t *testing.T) {
	_, err := newDaemonManager(daemonEnvironment{osName: "freebsd"}, &fakeCommandRunner{})
	if err == nil {
		t.Fatal("expected unsupported OS error")
	}
}

func TestDaemonMenuOptions(t *testing.T) {
	tests := []struct {
		name      string
		installed bool
		running   bool
		want      []string
	}{
		{
			name:      "not installed",
			installed: false,
			running:   false,
			want:      []string{"install", "exit"},
		},
		{
			name:      "installed stopped",
			installed: true,
			running:   false,
			want:      []string{"start", "uninstall", "exit"},
		},
		{
			name:      "installed running",
			installed: true,
			running:   true,
			want:      []string{"stop", "restart", "uninstall", "exit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := daemonMenuOptions(tt.installed, tt.running)
			if len(got) != len(tt.want) {
				t.Fatalf("len(daemonMenuOptions()) = %d, want %d", len(got), len(tt.want))
			}
			for i, want := range tt.want {
				if got[i].value != want {
					t.Fatalf("daemonMenuOptions()[%d] = %q, want %q", i, got[i].value, want)
				}
			}
		})
	}
}
