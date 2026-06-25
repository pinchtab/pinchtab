package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/daemon"
)

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

	printDaemonManageHints(st)
	printDaemonRecentLogs(st)
}

func printDaemonManageHints(st daemonStatus) {
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
}

func printDaemonRecentLogs(st daemonStatus) {
	if !st.Installed {
		return
	}
	logs := tailDaemonLogs()
	if logs == "" {
		return
	}
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Recent logs:"))
	for _, line := range strings.Split(logs, "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Printf("  %s\n", cli.StyleStdout(cli.MutedStyle, line))
		}
	}
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

// printDaemonActionError prints an action failure, the manager's manual
// fallback instructions, and exits — the shared error path for install and
// uninstall so their failure output cannot drift.
func printDaemonActionError(manager daemon.Manager, message string) {
	fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, message))
	fmt.Println()
	fmt.Println(manager.ManualInstructions())
	os.Exit(1)
}

func printDaemonFollowUp() {
	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Follow-up commands:"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon"), cli.StyleStdout(cli.MutedStyle, "# Check service health and logs"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon restart"), cli.StyleStdout(cli.MutedStyle, "# Apply config changes"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon stop"), cli.StyleStdout(cli.MutedStyle, "# Stop background service"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab daemon uninstall"), cli.StyleStdout(cli.MutedStyle, "# Remove service file"))
}
