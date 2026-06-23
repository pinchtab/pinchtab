package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type launchdManager struct {
	env    environment
	runner commandRunner
}

func (m *launchdManager) ServicePath() string {
	return filepath.Join(m.env.homeDir, "Library", "LaunchAgents", pinchtabLaunchdLabel+".plist")
}

func (m *launchdManager) Preflight() error {
	if strings.TrimSpace(m.env.userID) == "" {
		return fmt.Errorf("macOS daemon install requires a logged-in user session with a launchd GUI domain")
	}
	if _, err := runCommand(m.runner, "launchctl", "print", launchdDomainTarget(m.env)); err != nil {
		return fmt.Errorf("macOS daemon install requires an active launchd GUI session: %w", err)
	}
	return nil
}

func (m *launchdManager) Install(configPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(m.ServicePath()), 0755); err != nil {
		return "", fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := ensureDaemonLogDir(m.env); err != nil {
		return "", err
	}
	if err := os.WriteFile(m.ServicePath(), []byte(renderLaunchdPlist(m.env.execPath, configPath, m.env.homeDir, daemonStdoutLogPath(m.env), daemonStderrLogPath(m.env))), 0644); err != nil {
		return "", fmt.Errorf("write launchd plist: %w", err)
	}
	_, _ = runCommand(m.runner, "launchctl", "bootout", launchdDomainTarget(m.env), m.ServicePath())
	if _, err := runCommand(m.runner, "launchctl", "bootstrap", launchdDomainTarget(m.env), m.ServicePath()); err != nil {
		return "", err
	}
	if _, err := runCommand(m.runner, "launchctl", "kickstart", "-k", launchdDomainTarget(m.env)+"/"+pinchtabLaunchdLabel); err != nil {
		return "", err
	}
	return fmt.Sprintf("Installed launchd agent at %s", m.ServicePath()), nil
}

func (m *launchdManager) Start() (string, error) {
	if err := ensureDaemonLogDir(m.env); err != nil {
		return "", err
	}
	if _, err := runCommand(m.runner, "launchctl", "bootstrap", launchdDomainTarget(m.env), m.ServicePath()); err != nil && !isLaunchdAlreadyBootstrapped(err) {
		return "", err
	}
	if _, err := runCommand(m.runner, "launchctl", "kickstart", launchdDomainTarget(m.env)+"/"+pinchtabLaunchdLabel); err != nil {
		return "", err
	}
	return "Pinchtab daemon started.", nil
}

func (m *launchdManager) Restart() (string, error) {
	if err := ensureDaemonLogDir(m.env); err != nil {
		return "", err
	}
	if _, err := runCommand(m.runner, "launchctl", "kickstart", "-k", launchdDomainTarget(m.env)+"/"+pinchtabLaunchdLabel); err != nil {
		return "", err
	}
	return "Pinchtab daemon restarted.", nil
}

func (m *launchdManager) Stop() (string, error) {
	_, err := runCommand(m.runner, "launchctl", "bootout", launchdDomainTarget(m.env), m.ServicePath())
	if err != nil && !isLaunchdIgnorableError(err) {
		return "", err
	}
	return "Pinchtab daemon stopped.", nil
}

func (m *launchdManager) Status() (string, error) {
	output, err := runCommand(m.runner, "launchctl", "print", launchdDomainTarget(m.env)+"/"+pinchtabLaunchdLabel)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(output) == "" {
		return "Pinchtab daemon status returned no output.", nil
	}
	return output, nil
}

func (m *launchdManager) Uninstall() (string, error) {
	var errs []error
	_, err := runCommand(m.runner, "launchctl", "bootout", launchdDomainTarget(m.env), m.ServicePath())
	if err != nil && !isLaunchdIgnorableError(err) {
		errs = append(errs, err)
	}
	if err := os.Remove(m.ServicePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("remove launchd plist: %w", err))
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	return "Pinchtab daemon uninstalled.", nil
}

func (m *launchdManager) Pid() (string, error) {
	output, err := runCommand(m.runner, "launchctl", "print", launchdDomainTarget(m.env)+"/"+pinchtabLaunchdLabel)
	if err != nil {
		return "", err
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pid = ") {
			return strings.TrimPrefix(trimmed, "pid = "), nil
		}
	}
	return "", nil
}

func (m *launchdManager) Logs(n int) (string, error) {
	logPath := daemonStderrLogPath(m.env)
	if info, err := os.Stat(logPath); err == nil && info.Size() > 0 {
		return runCommand(m.runner, "tail", "-n", fmt.Sprintf("%d", n), logPath)
	}

	legacyLogPath := "/tmp/pinchtab.err.log"
	if _, err := os.Stat(legacyLogPath); err == nil {
		return runCommand(m.runner, "tail", "-n", fmt.Sprintf("%d", n), legacyLogPath)
	}

	return "No logs found at " + logPath, nil
}

func (m *launchdManager) ManualInstructions() string {
	path := m.ServicePath()
	target := launchdDomainTarget(m.env)
	var b strings.Builder
	fmt.Fprintln(&b, "Manual instructions (macOS/launchd):")
	fmt.Fprintln(&b, "To install manually:")
	fmt.Fprintf(&b, "  1. Create %s\n", path)
	fmt.Fprintf(&b, "  2. Run: launchctl bootstrap %s %s\n", target, path)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "To uninstall manually:")
	fmt.Fprintf(&b, "  1. Run: launchctl bootout %s %s\n", target, path)
	fmt.Fprintf(&b, "  2. Remove: %s\n", path)
	return b.String()
}

// launchctl error-message substrings treated as benign. "already bootstrapped"
// is shared with isLaunchdAlreadyBootstrapped so the literal lives in one place.
const (
	launchdAlreadyBootstrapped = "already bootstrapped"
	launchdExitStatus5         = "exit status 5"
	launchdNoSuchProcess       = "No such process"
	launchdNotFound            = "not found"
)

// isLaunchdAlreadyBootstrapped reports whether a launchctl error means the
// service is already bootstrapped — for Start, that makes bootstrap an
// effective no-op success. It is deliberately narrower than
// isLaunchdIgnorableError (which also tolerates the teardown "service absent"
// cases that a failed Start must NOT swallow).
func isLaunchdAlreadyBootstrapped(err error) bool {
	return err != nil && strings.Contains(err.Error(), launchdAlreadyBootstrapped)
}

func isLaunchdIgnorableError(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, launchdExitStatus5) ||
		strings.Contains(msg, launchdNoSuchProcess) ||
		strings.Contains(msg, launchdNotFound) ||
		strings.Contains(msg, launchdAlreadyBootstrapped)
}

func renderLaunchdPlist(execPath, configPath, homeDir, stdoutPath, stderrPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>server</string>
  </array>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ExitTimeOut</key>
  <integer>10</integer>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
    <key>PINCHTAB_CONFIG</key>
    <string>%s</string>
  </dict>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
	`, pinchtabLaunchdLabel, execPath, homeDir, homeDir, configPath, stdoutPath, stderrPath)
}
