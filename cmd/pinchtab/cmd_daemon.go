package main

import (
	"encoding/json"
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
		jsonOut, _ := cmd.Flags().GetBool("json")
		sub := ""
		if len(args) > 0 {
			sub = args[0]
		}
		handleDaemonCommand(sub, jsonOut)
	},
}

func init() {
	daemonCmd.GroupID = "primary"
	daemonCmd.Flags().Bool("json", false, "Print daemon status as JSON (status only, no actions)")
	rootCmd.AddCommand(daemonCmd)
}

func handleDaemonCommand(subcommand string, jsonOut bool) {
	if subcommand == "" || subcommand == "help" || subcommand == "--help" || subcommand == "-h" {
		if jsonOut {
			printDaemonStatusJSON()
			return
		}
		printDaemonOverview()
		return
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
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.MutedStyle, "Usage: pinchtab daemon <install|start|restart|stop|uninstall>"))
		os.Exit(2)
	}
}

type daemonStatus struct {
	Installed      bool   `json:"installed"`
	Running        bool   `json:"running"`
	PID            string `json:"pid,omitempty"`
	ServicePath    string `json:"servicePath,omitempty"`
	PreflightError string `json:"preflightError,omitempty"`
	ManagerError   string `json:"managerError,omitempty"`
}

func collectDaemonStatus() daemonStatus {
	st := daemonStatus{
		Installed: daemon.IsInstalled(),
		Running:   daemon.IsRunning(),
	}
	manager, err := daemon.CurrentManager()
	if err != nil {
		st.ManagerError = err.Error()
		return st
	}
	if st.Running {
		if pid, err := manager.Pid(); err == nil {
			st.PID = pid
		}
	}
	if st.Installed {
		st.ServicePath = manager.ServicePath()
	}
	if err := manager.Preflight(); err != nil {
		st.PreflightError = err.Error()
	}
	return st
}

func printDaemonStatusJSON() {
	st := collectDaemonStatus()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(st)
}

func printDaemonOverview() {
	st := collectDaemonStatus()

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Daemon"))
	fmt.Println()

	if st.ManagerError != "" {
		fmt.Printf("  %-20s %s\n", "manager", cli.StyleStdout(cli.ErrorStyle, st.ManagerError))
		fmt.Println()
		return
	}

	serviceVal, serviceStyle := "not installed", cli.WarningStyle
	if st.Installed {
		serviceVal, serviceStyle = "installed", cli.SuccessStyle
	}
	fmt.Printf("  %-20s %s\n", "service", cli.StyleStdout(serviceStyle, serviceVal))

	stateVal, stateStyle := "stopped", cli.WarningStyle
	if st.Running {
		stateVal, stateStyle = "running", cli.SuccessStyle
	}
	fmt.Printf("  %-20s %s\n", "state", cli.StyleStdout(stateStyle, stateVal))

	if st.PID != "" {
		fmt.Printf("  %-20s %s\n", "pid", cli.StyleStdout(cli.ValueStyle, st.PID))
	}
	if st.ServicePath != "" {
		fmt.Printf("  %-20s %s\n", "path", cli.StyleStdout(cli.ValueStyle, st.ServicePath))
	}
	if st.PreflightError != "" {
		fmt.Printf("  %-20s %s\n", "environment", cli.StyleStdout(cli.WarningStyle, st.PreflightError))
	}
	fmt.Println()

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Manage daemon:"))
	switch {
	case !st.Installed:
		fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon install"), cli.StyleStdout(cli.MutedStyle, "# install background service"))
	case st.Running:
		fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon stop"), cli.StyleStdout(cli.MutedStyle, "# stop the service"))
		fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon restart"), cli.StyleStdout(cli.MutedStyle, "# restart (apply config changes)"))
		fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon uninstall"), cli.StyleStdout(cli.MutedStyle, "# remove service file"))
	default:
		fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon start"), cli.StyleStdout(cli.MutedStyle, "# start the service"))
		fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon uninstall"), cli.StyleStdout(cli.MutedStyle, "# remove service file"))
	}
	fmt.Printf("  %-44s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon --json"), cli.StyleStdout(cli.MutedStyle, "# status as JSON"))

	if st.Installed {
		if logs := tailDaemonLogs(); logs != "" {
			fmt.Println()
			fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Recent logs:"))
			for _, line := range strings.Split(logs, "\n") {
				if strings.TrimSpace(line) != "" {
					fmt.Printf("  %s\n", cli.StyleStdout(cli.MutedStyle, line))
				}
			}
		}
	}
}

func tailDaemonLogs() string {
	manager, err := daemon.CurrentManager()
	if err != nil {
		return ""
	}
	logs, err := manager.Logs(5)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(logs)
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
