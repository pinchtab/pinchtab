package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	pinchtabDaemonUnitName = "pinchtab.service"
	pinchtabLaunchdLabel   = "com.pinchtab.pinchtab"
)

type Manager interface {
	Preflight() error
	Install(configPath string) (string, error)
	ServicePath() string
	Start() (string, error)
	Restart() (string, error)
	Status() (string, error)
	Stop() (string, error)
	Uninstall() (string, error)
	ManualInstructions() string
	Pid() (string, error)
	Logs(n int) (string, error)
}

type commandRunner interface {
	CombinedOutput(name string, arg ...string) ([]byte, error)
}

type osCommandRunner struct{}

func (r osCommandRunner) CombinedOutput(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).CombinedOutput() // #nosec G204 -- args are daemon manager controlled, not user input
}

type environment struct {
	execPath      string
	homeDir       string
	osName        string
	userID        string
	xdgConfigHome string
}

func CurrentManager() (Manager, error) {
	env, err := currentEnvironment()
	if err != nil {
		return nil, err
	}
	return newManager(env, osCommandRunner{})
}

func IsInstalled() bool {
	manager, err := CurrentManager()
	if err != nil {
		return false
	}
	_, err = os.Stat(manager.ServicePath())
	return err == nil
}

func IsRunning() bool {
	manager, err := CurrentManager()
	if err != nil {
		return false
	}
	status, err := manager.Status()
	if err != nil {
		return false
	}
	return StatusLooksRunning(status)
}

func StatusLooksRunning(status string) bool {
	return strings.Contains(status, "state = running") ||
		strings.Contains(status, "Active: active (running)")
}

func currentEnvironment() (environment, error) {
	execPath, err := os.Executable()
	if err != nil {
		return environment{}, fmt.Errorf("resolve executable path: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return environment{}, fmt.Errorf("resolve home directory: %w", err)
	}
	currentUser, err := user.Current()
	if err != nil {
		return environment{}, fmt.Errorf("resolve current user: %w", err)
	}

	return environment{
		execPath:      execPath,
		homeDir:       homeDir,
		osName:        runtime.GOOS,
		userID:        currentUser.Uid,
		xdgConfigHome: os.Getenv("XDG_CONFIG_HOME"),
	}, nil
}

func newManager(env environment, runner commandRunner) (Manager, error) {
	switch env.osName {
	case "linux":
		return &systemdUserManager{env: env, runner: runner}, nil
	case "darwin":
		return &launchdManager{env: env, runner: runner}, nil
	default:
		return nil, fmt.Errorf("pinchtab daemon is supported on macOS and Linux; current OS is %s", env.osName)
	}
}

func runCommand(runner commandRunner, name string, args ...string) (string, error) {
	output, err := runner.CombinedOutput(name, args...)
	trimmed := strings.TrimSpace(string(output))
	if err == nil {
		return trimmed, nil
	}
	if trimmed == "" {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, trimmed)
}

func launchdDomainTarget(env environment) string {
	return "gui/" + env.userID
}

func systemdUserConfigHome(env environment) string {
	if strings.TrimSpace(env.xdgConfigHome) != "" {
		return env.xdgConfigHome
	}
	return filepath.Join(env.homeDir, ".config")
}
