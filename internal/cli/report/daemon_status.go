package report

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

var daemonExecCommand = exec.Command

func IsDaemonInstalled() bool {
	manager, err := currentDaemonManager()
	if err != nil {
		return false
	}
	_, err = os.Stat(manager.ServicePath())
	return err == nil
}

func IsDaemonRunning() bool {
	manager, err := currentDaemonManager()
	if err != nil {
		return false
	}
	status, err := manager.Status()
	if err != nil {
		return false
	}
	return daemonStatusLooksRunning(status)
}

// Internal copy of simplified daemon manager discovery for status checks
type daemonInfo struct {
	osName string
	userID string
}

func currentDaemonManager() (daemonService, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	info := daemonInfo{osName: runtime.GOOS, userID: u.Uid}
	switch info.osName {
	case "linux":
		return &systemdStatus{userID: u.Uid}, nil
	case "darwin":
		return &launchdStatus{userID: u.Uid}, nil
	case "windows":
		return &windowsTaskStatus{}, nil
	default:
		return nil, fmt.Errorf("unsupported OS")
	}
}

type daemonService interface {
	ServicePath() string
	Status() (string, error)
}

type systemdStatus struct{ userID string }

func (s *systemdStatus) ServicePath() string {
	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		return filepath.Join(configHome, "systemd", "user", "pinchtab.service")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "pinchtab.service")
}
func (s *systemdStatus) Status() (string, error) {
	return runDaemonCommand("systemctl", "--user", "status", "pinchtab.service", "--no-pager")
}

type launchdStatus struct{ userID string }

func (s *launchdStatus) ServicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.pinchtab.pinchtab.plist")
}
func (s *launchdStatus) Status() (string, error) {
	return runDaemonCommand("launchctl", "print", "gui/"+s.userID+"/com.pinchtab.pinchtab")
}

func runDaemonCommand(name string, args ...string) (string, error) {
	cmd := daemonExecCommand(name, args...)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err == nil {
		return trimmed, nil
	}
	if trimmed == "" {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, trimmed)
}

type windowsTaskStatus struct{}

func (w *windowsTaskStatus) ServicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pinchtab", "pinchtab-daemon.cmd")
}

func (w *windowsTaskStatus) Status() (string, error) {
	return runDaemonCommand("schtasks.exe", "/Query", "/TN", "PinchTab", "/V", "/FO", "LIST")
}

func daemonStatusLooksRunning(status string) bool {
	return strings.Contains(status, "state = running") ||
		strings.Contains(status, "Active: active (running)") ||
		strings.Contains(status, "Status:                          Running")
}
