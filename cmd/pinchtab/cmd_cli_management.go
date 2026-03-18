package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/urls"
	"github.com/spf13/cobra"
)

var instancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "List or manage instances",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Instances(rt.client, rt.base, rt.token)
		})
	},
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check server health",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Health(rt.client, rt.base, rt.token)
		})
	},
}

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List browser profiles",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Profiles(rt.client, rt.base, rt.token)
		})
	},
}

var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "List recorded activity events",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Activity(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var activityTabCmd = &cobra.Command{
	Use:   "tab <id>",
	Short: "List recorded activity events for a tab",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.ActivityTab(rt.client, rt.base, rt.token, args[0], cmd)
		})
	},
}

var instanceCmd = &cobra.Command{
	Use:   "instance",
	Short: "Manage browser instances",
}

var startInstanceCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a browser instance",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.InstanceStart(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var instanceNavigateCmd = &cobra.Command{
	Use:   "navigate <id> <url>",
	Short: "Navigate an instance to a URL",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		args[1] = urls.Normalize(args[1])
		runCLI(func(rt cliRuntime) {
			browseractions.InstanceNavigate(rt.client, rt.base, rt.token, args)
		})
	},
}

var instanceStopCmd = &cobra.Command{
	Use:   "stop <id>",
	Short: "Stop a browser instance",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.InstanceStop(rt.client, rt.base, rt.token, args)
		})
	},
}

var instanceLogsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "Get instance logs",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.InstanceLogs(rt.client, rt.base, rt.token, args)
		})
	},
}
