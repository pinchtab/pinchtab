package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/spf13/cobra"
)

const (
	pinchtabDaemonUnitName = "pinchtab.service"
	pinchtabLaunchdLabel   = "com.pinchtab.pinchtab"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon [action]",
	Short: "Manage the background service",
	Long:  "Start, stop, install, or check the status of the PinchTab background service.",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Load()
		sub := ""
		if len(args) > 0 {
			sub = args[0]
		}
		handleDaemonCommand(cfg, sub)
	},
}

func init() {
	daemonCmd.GroupID = "primary"
	rootCmd.AddCommand(daemonCmd)
}

func handleDaemonCommand(_ *config.RuntimeConfig, subcommand string) {
	if subcommand == "" || subcommand == "help" || subcommand == "--help" || subcommand == "-h" {
		printDaemonStatusSummary()

		if subcommand == "" && isInteractiveTerminal() {
			picked, err := promptSelect("Daemon Actions", daemonMenuOptions(cli.IsDaemonInstalled(), cli.IsDaemonRunning()))
			if err != nil || picked == "exit" || picked == "" {
				os.Exit(0)
			}
			subcommand = picked
		} else {
			daemonUsage()
			if subcommand == "" {
				os.Exit(0)
			}
			return
		}
	}

	manager, err := currentDaemonManager()
	if err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}

	switch subcommand {
	case "install":
		configPath, fileCfg, _, err := ensureDaemonConfig(false)
		if err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("daemon install failed: %v", err)))
			os.Exit(1)
		}
		// Run wizard if needed (first install or version upgrade)
		if config.NeedsWizard(fileCfg) {
			isNew := config.IsFirstRun(fileCfg)
			runSecurityWizard(fileCfg, configPath, isNew)
		}
		if err := manager.Preflight(); err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("daemon install unavailable: %v", err)))
			os.Exit(1)
		}
		message, err := manager.Install(managerEnvironment(manager).execPath, configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("daemon install failed: %v", err)))
			fmt.Println()
			fmt.Println(manager.ManualInstructions())
			os.Exit(1)
		}
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "  [ok] ") + message)
		printDaemonFollowUp()
	case "start":
		printDaemonManagerResult(manager.Start())
	case "restart":
		printDaemonManagerResult(manager.Restart())
	case "stop":
		printDaemonManagerResult(manager.Stop())
	case "uninstall":
		message, err := manager.Uninstall()
		if err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
			fmt.Println()
			fmt.Println(manager.ManualInstructions())
			os.Exit(1)
		}
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "  [ok] ") + message)
	default:
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("unknown daemon command: %s", subcommand)))
		daemonUsage()
		os.Exit(2)
	}
}

func daemonUsage() {
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Usage:") + " " + cli.StyleStdout(cli.CommandStyle, "pinchtab daemon <install|start|restart|stop|uninstall>"))
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.MutedStyle, "Manage the PinchTab user-level background service."))
	fmt.Println()
}

func daemonMenuOptions(installed, running bool) []menuOption {
	options := make([]menuOption, 0, 4)
	switch {
	case !installed:
		options = append(options, menuOption{label: "Install service", value: "install"})
	case running:
		options = append(options,
			menuOption{label: "Stop service", value: "stop"},
			menuOption{label: "Restart service", value: "restart"},
			menuOption{label: "Uninstall service", value: "uninstall"},
		)
	default:
		options = append(options,
			menuOption{label: "Start service", value: "start"},
			menuOption{label: "Uninstall service", value: "uninstall"},
		)
	}
	options = append(options, menuOption{label: "Exit", value: "exit"})
	return options
}

func printDaemonStatusSummary() {
	manager, err := currentDaemonManager()
	if err != nil {
		fmt.Println(cli.StyleStdout(cli.ErrorStyle, "  Error: ") + err.Error())
		return
	}

	installed := cli.IsDaemonInstalled()
	running := cli.IsDaemonRunning()

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Daemon status:"))

	status := cli.StyleStdout(cli.WarningStyle, "not installed")
	if installed {
		status = cli.StyleStdout(cli.SuccessStyle, "installed")
	}
	fmt.Printf("  %-12s %s\n", cli.StyleStdout(cli.MutedStyle, "Service:"), status)

	state := cli.StyleStdout(cli.MutedStyle, "stopped")
	if running {
		state = cli.StyleStdout(cli.SuccessStyle, "active (running)")
	}
	fmt.Printf("  %-12s %s\n", cli.StyleStdout(cli.MutedStyle, "State:"), state)

	if running {
		pid, _ := manager.Pid()
		if pid != "" {
			fmt.Printf("  %-12s %s\n", cli.StyleStdout(cli.MutedStyle, "PID:"), cli.StyleStdout(cli.ValueStyle, pid))
		}
	}

	if installed {
		fmt.Printf("  %-12s %s\n", cli.StyleStdout(cli.MutedStyle, "Path:"), cli.StyleStdout(cli.ValueStyle, manager.ServicePath()))
	}
	if err := manager.Preflight(); err != nil {
		fmt.Printf("  %-12s %s\n", cli.StyleStdout(cli.MutedStyle, "Environment:"), cli.StyleStdout(cli.WarningStyle, err.Error()))
	}

	if installed {
		logs, err := manager.Logs(5)
		if err == nil && strings.TrimSpace(logs) != "" {
			fmt.Println()
			fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Recent logs:"))
			lines := strings.Split(logs, "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					fmt.Printf("  %s\n", cli.StyleStdout(cli.MutedStyle, line))
				}
			}
		}
	}
	fmt.Println()
}

func printDaemonManagerResult(message string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}
	if strings.HasPrefix(message, "Installed") || strings.HasPrefix(message, "Pinchtab daemon") {
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, "  [ok] ") + message)
	} else {
		// For status, it might be a block of text
		fmt.Println(message)
	}
}

func ensureDaemonConfig(force bool) (string, *config.FileConfig, configBootstrapStatus, error) {
	_, configPath, err := config.LoadFileConfig()
	if err != nil {
		return "", nil, "", err
	}

	exists := fileExists(configPath)
	if !exists || force {
		defaults := config.DefaultFileConfig()
		defaults.ConfigVersion = "" // Leave empty so wizard triggers on first install
		token, err := config.GenerateAuthToken()
		if err != nil {
			return "", nil, "", err
		}
		defaults.Server.Token = token
		if err := config.SaveFileConfig(&defaults, configPath); err != nil {
			return "", nil, "", err
		}
		status := configCreated
		if exists {
			status = configRecovered
		}
		return configPath, &defaults, status, nil
	}

	// File exists — load it as-is, don't overwrite security settings.
	// Security recovery is now handled by the wizard or `pinchtab security up`.
	fileCfg, _, _ := config.LoadFileConfig()
	if fileCfg == nil {
		return configPath, nil, "", fmt.Errorf("failed to load existing config at %s", configPath)
	}

	// Only generate a token if one is missing (security essential)
	if strings.TrimSpace(fileCfg.Server.Token) == "" {
		token, err := config.GenerateAuthToken()
		if err == nil {
			fileCfg.Server.Token = token
			_ = config.SaveFileConfig(fileCfg, configPath)
			return configPath, fileCfg, configRecovered, nil
		}
	}

	return configPath, fileCfg, configVerified, nil
}

type configBootstrapStatus string

const (
	configCreated   configBootstrapStatus = "created"
	configRecovered configBootstrapStatus = "recovered"
	configVerified  configBootstrapStatus = "verified"
)

type commandRunner interface {
	CombinedOutput(name string, arg ...string) ([]byte, error)
}

type osCommandRunner struct{}

func (r osCommandRunner) CombinedOutput(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).CombinedOutput() // #nosec G204 -- args are daemon manager controlled, not user input
}

type daemonEnvironment struct {
	execPath      string
	homeDir       string
	osName        string
	userID        string
	xdgConfigHome string
}

type daemonManager interface {
	Preflight() error
	Install(execPath, configPath string) (string, error)
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

type systemdUserManager struct {
	env    daemonEnvironment
	runner commandRunner
}

type launchdManager struct {
	env    daemonEnvironment
	runner commandRunner
}

func printDaemonFollowUp() {
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Follow-up commands:"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon"), cli.StyleStdout(cli.MutedStyle, "# Check service health and logs"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon restart"), cli.StyleStdout(cli.MutedStyle, "# Apply config changes"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon stop"), cli.StyleStdout(cli.MutedStyle, "# Stop background service"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon uninstall"), cli.StyleStdout(cli.MutedStyle, "# Remove service file"))
}

func currentDaemonManager() (daemonManager, error) {
	env, err := currentDaemonEnvironment()
	if err != nil {
		return nil, err
	}
	return newDaemonManager(env, osCommandRunner{})
}

func currentDaemonEnvironment() (daemonEnvironment, error) {
	execPath, err := os.Executable()
	if err != nil {
		return daemonEnvironment{}, fmt.Errorf("resolve executable path: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return daemonEnvironment{}, fmt.Errorf("resolve home directory: %w", err)
	}
	currentUser, err := user.Current()
	if err != nil {
		return daemonEnvironment{}, fmt.Errorf("resolve current user: %w", err)
	}

	return daemonEnvironment{
		execPath:      execPath,
		homeDir:       homeDir,
		osName:        runtime.GOOS,
		userID:        currentUser.Uid,
		xdgConfigHome: os.Getenv("XDG_CONFIG_HOME"),
	}, nil
}

func newDaemonManager(env daemonEnvironment, runner commandRunner) (daemonManager, error) {
	return newPlatformDaemonManager(env, runner)
}

func managerEnvironment(manager daemonManager) daemonEnvironment {
	return managerEnvironmentPlatform(manager)
}

func (m *systemdUserManager) ServicePath() string {
	return filepath.Join(systemdUserConfigHome(m.env), "systemd", "user", pinchtabDaemonUnitName)
}

func (m *systemdUserManager) Preflight() error {
	if _, err := runCommand(m.runner, "systemctl", "--user", "show-environment"); err != nil {
		return fmt.Errorf("linux daemon install requires a working user systemd session (`systemctl --user`): %w", err)
	}
	return nil
}

func (m *systemdUserManager) Install(execPath, configPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(m.ServicePath()), 0755); err != nil {
		return "", fmt.Errorf("create systemd user directory: %w", err)
	}
	if err := os.WriteFile(m.ServicePath(), []byte(renderSystemdUnit(execPath, configPath)), 0644); err != nil {
		return "", fmt.Errorf("write systemd unit: %w", err)
	}
	if _, err := runCommand(m.runner, "systemctl", "--user", "daemon-reload"); err != nil {
		return "", err
	}
	if _, err := runCommand(m.runner, "systemctl", "--user", "enable", "--now", pinchtabDaemonUnitName); err != nil {
		return "", err
	}
	return fmt.Sprintf("Installed systemd user service at %s", m.ServicePath()), nil
}

func (m *systemdUserManager) Start() (string, error) {
	if _, err := runCommand(m.runner, "systemctl", "--user", "start", pinchtabDaemonUnitName); err != nil {
		return "", err
	}
	return "Pinchtab daemon started.", nil
}

func (m *systemdUserManager) Restart() (string, error) {
	if _, err := runCommand(m.runner, "systemctl", "--user", "restart", pinchtabDaemonUnitName); err != nil {
		return "", err
	}
	return "Pinchtab daemon restarted.", nil
}

func (m *systemdUserManager) Stop() (string, error) {
	if _, err := runCommand(m.runner, "systemctl", "--user", "stop", pinchtabDaemonUnitName); err != nil {
		return "", err
	}
	return "Pinchtab daemon stopped.", nil
}

func (m *systemdUserManager) Status() (string, error) {
	output, err := runCommand(m.runner, "systemctl", "--user", "status", pinchtabDaemonUnitName, "--no-pager")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(output) == "" {
		return "Pinchtab daemon status returned no output.", nil
	}
	return output, nil
}

func (m *systemdUserManager) Uninstall() (string, error) {
	var errs []error
	if _, err := runCommand(m.runner, "systemctl", "--user", "disable", "--now", pinchtabDaemonUnitName); err != nil {
		errs = append(errs, err)
	}
	if err := os.Remove(m.ServicePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("remove unit file: %w", err))
	}
	if _, err := runCommand(m.runner, "systemctl", "--user", "daemon-reload"); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	return "Pinchtab daemon uninstalled.", nil
}

func (m *systemdUserManager) Pid() (string, error) {
	output, err := runCommand(m.runner, "systemctl", "--user", "show", pinchtabDaemonUnitName, "--property", "MainPID")
	if err != nil {
		return "", err
	}
	// Output is typically MainPID=1234
	if parts := strings.Split(output, "="); len(parts) == 2 {
		pid := strings.TrimSpace(parts[1])
		if pid == "0" {
			return "", nil // Not running
		}
		return pid, nil
	}
	return "", nil
}

func (m *systemdUserManager) Logs(n int) (string, error) {
	return runCommand(m.runner, "journalctl", "--user", "-u", pinchtabDaemonUnitName, "-n", fmt.Sprintf("%d", n), "--no-pager")
}

func (m *systemdUserManager) ManualInstructions() string {
	path := m.ServicePath()
	var b strings.Builder
	fmt.Fprintln(&b, cli.StyleStdout(cli.HeadingStyle, "Manual instructions (Linux/systemd):"))
	fmt.Fprintln(&b, cli.StyleStdout(cli.MutedStyle, "To install manually:"))
	fmt.Fprintf(&b, "  1. Create %s\n", cli.StyleStdout(cli.ValueStyle, path))
	fmt.Fprintln(&b, "  2. Run: "+cli.StyleStdout(cli.CommandStyle, "systemctl --user daemon-reload"))
	fmt.Fprintln(&b, "  3. Run: "+cli.StyleStdout(cli.CommandStyle, "systemctl --user enable --now pinchtab.service"))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, cli.StyleStdout(cli.MutedStyle, "To uninstall manually:"))
	fmt.Fprintln(&b, "  1. Run: "+cli.StyleStdout(cli.CommandStyle, "systemctl --user disable --now pinchtab.service"))
	fmt.Fprintf(&b, "  2. Remove: %s\n", cli.StyleStdout(cli.ValueStyle, path))
	fmt.Fprintln(&b, "  3. Run: "+cli.StyleStdout(cli.CommandStyle, "systemctl --user daemon-reload"))
	return b.String()
}

func renderSystemdUnit(execPath, configPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Pinchtab Browser Service
After=network.target

[Service]
Type=simple
ExecStart="%s" server
Environment="PINCHTAB_CONFIG=%s"
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, execPath, configPath)
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

func (m *launchdManager) Install(execPath, configPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(m.ServicePath()), 0755); err != nil {
		return "", fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := os.WriteFile(m.ServicePath(), []byte(renderLaunchdPlist(execPath, configPath)), 0644); err != nil {
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
	if _, err := runCommand(m.runner, "launchctl", "bootstrap", launchdDomainTarget(m.env), m.ServicePath()); err != nil && !strings.Contains(err.Error(), "already bootstrapped") {
		return "", err
	}
	if _, err := runCommand(m.runner, "launchctl", "kickstart", launchdDomainTarget(m.env)+"/"+pinchtabLaunchdLabel); err != nil {
		return "", err
	}
	return "Pinchtab daemon started.", nil
}

func (m *launchdManager) Restart() (string, error) {
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
	// Try to find pid = 1234
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
	// macOS log paths we added to plist
	logPath := "/tmp/pinchtab.err.log"
	if _, err := os.Stat(logPath); err != nil {
		return "No logs found at " + logPath, nil
	}
	return runCommand(m.runner, "tail", "-n", fmt.Sprintf("%d", n), logPath)
}

func (m *launchdManager) ManualInstructions() string {
	path := m.ServicePath()
	target := launchdDomainTarget(m.env)
	var b strings.Builder
	fmt.Fprintln(&b, cli.StyleStdout(cli.HeadingStyle, "Manual instructions (macOS/launchd):"))
	fmt.Fprintln(&b, cli.StyleStdout(cli.MutedStyle, "To install manually:"))
	fmt.Fprintf(&b, "  1. Create %s\n", cli.StyleStdout(cli.ValueStyle, path))
	fmt.Fprintln(&b, "  2. Run: "+cli.StyleStdout(cli.CommandStyle, fmt.Sprintf("launchctl bootstrap %s %s", target, path)))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, cli.StyleStdout(cli.MutedStyle, "To uninstall manually:"))
	fmt.Fprintln(&b, "  1. Run: "+cli.StyleStdout(cli.CommandStyle, fmt.Sprintf("launchctl bootout %s %s", target, path)))
	fmt.Fprintf(&b, "  2. Remove: %s\n", cli.StyleStdout(cli.ValueStyle, path))
	return b.String()
}

func isLaunchdIgnorableError(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	// Exit status 5 often means already booted out or path not found in domain
	// "No such process" (status 3) or "No such file or directory" (status 2) or "Operation not permitted" (sometimes)
	return strings.Contains(msg, "exit status 5") ||
		strings.Contains(msg, "No such process") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "already bootstrapped")
}

func renderLaunchdPlist(execPath, configPath string) string {
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
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ExitTimeOut</key>
  <integer>10</integer>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PINCHTAB_CONFIG</key>
    <string>%s</string>
  </dict>
  <key>StandardOutPath</key>
  <string>/tmp/pinchtab.out.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/pinchtab.err.log</string>
</dict>
</plist>
`, pinchtabLaunchdLabel, execPath, configPath)
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

func launchdDomainTarget(env daemonEnvironment) string {
	return "gui/" + env.userID
}

func systemdUserConfigHome(env daemonEnvironment) string {
	if strings.TrimSpace(env.xdgConfigHome) != "" {
		return env.xdgConfigHome
	}
	return filepath.Join(env.homeDir, ".config")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isInteractiveTerminal() bool {
	in, err := os.Stdin.Stat()
	if err != nil || (in.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	out, err := os.Stdout.Stat()
	if err != nil || (out.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	return true
}
