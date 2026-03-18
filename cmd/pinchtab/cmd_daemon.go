package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/daemon"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon [action]",
	Short: "Manage the background service",
	Long:  "Start, stop, install, or check the status of the PinchTab background service.",
	Run: func(cmd *cobra.Command, args []string) {
		sub := ""
		if len(args) > 0 {
			sub = args[0]
		}
		handleDaemonCommand(sub)
	},
}

func init() {
	daemonCmd.GroupID = "primary"
	rootCmd.AddCommand(daemonCmd)
}

func handleDaemonCommand(subcommand string) {
	if subcommand == "" || subcommand == "help" || subcommand == "--help" || subcommand == "-h" {
		printDaemonStatusSummary()

		if subcommand == "" && isInteractiveTerminal() {
			picked, err := promptSelect("Daemon Actions", daemonMenuOptions(daemon.IsInstalled(), daemon.IsRunning()))
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

	manager, err := daemon.CurrentManager()
	if err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, err.Error()))
		os.Exit(1)
	}

	switch subcommand {
	case "install":
		configPath, fileCfg, _, err := daemon.EnsureConfig(false)
		if err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("daemon install failed: %v", err)))
			os.Exit(1)
		}
		if config.NeedsWizard(fileCfg) {
			isNew := config.IsFirstRun(fileCfg)
			runSecurityWizard(fileCfg, configPath, isNew)
		}
		if err := manager.Preflight(); err != nil {
			fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("daemon install unavailable: %v", err)))
			os.Exit(1)
		}
		message, err := manager.Install(configPath)
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
	manager, err := daemon.CurrentManager()
	if err != nil {
		fmt.Println(cli.StyleStdout(cli.ErrorStyle, "  Error: ") + err.Error())
		return
	}

	installed := daemon.IsInstalled()
	running := daemon.IsRunning()

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
		return
	}
	fmt.Println(message)
}

func printDaemonFollowUp() {
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Follow-up commands:"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon"), cli.StyleStdout(cli.MutedStyle, "# Check service health and logs"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon restart"), cli.StyleStdout(cli.MutedStyle, "# Apply config changes"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon stop"), cli.StyleStdout(cli.MutedStyle, "# Stop background service"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon uninstall"), cli.StyleStdout(cli.MutedStyle, "# Remove service file"))
}
