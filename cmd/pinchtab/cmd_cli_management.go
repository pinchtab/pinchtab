package main

import (
	"fmt"

	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/urls"
	"github.com/spf13/cobra"
)

func listInstances(cmd *cobra.Command) {
	runCLI(func(rt cliRuntime) {
		browseractions.Instances(rt.client, rt.base, rt.token, cmd)
	})
}

var instanceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List browser instances",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		listInstances(cmd)
	},
}

// instancesCmd is the pre-`instance list` spelling. It stays runnable so
// existing scripts keep working, and hidden so the root listing no longer
// offers two commands one character apart for the same subject.
var instancesCmd = &cobra.Command{
	Use:        "instances",
	Short:      "List browser instances (deprecated: use \"pinchtab instance list\")",
	Hidden:     true,
	Deprecated: "use \"pinchtab instance list\" instead.",
	Run: func(cmd *cobra.Command, args []string) {
		listInstances(cmd)
	},
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check server health",
	Run: func(cmd *cobra.Command, args []string) {
		config.EmitDefaultConfigHint()
		runCLI(func(rt cliRuntime) {
			browseractions.Health(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List browser profiles",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Profiles(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var profilesPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Reclaim disk from quarantined profiles (lists only unless --confirm)",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.ProfilesPrune(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var profilesCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a browser profile for human setup and authentication",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.ProfilesCreate(rt.client, rt.base, rt.token, args[0])
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

// instanceIDArgs replaces cobra's "accepts 1 arg(s), received 0" on the
// subcommands that need an instance id. The id only comes from one place, so
// the error says where rather than leaving the operator to find it.
func instanceIDArgs(count int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == count {
			return nil
		}
		return fmt.Errorf("%s needs %s; run \"pinchtab instance list\" to see running instances and their ids",
			cmd.CommandPath(), pluralArgs(count))
	}
}

func pluralArgs(count int) string {
	if count == 1 {
		return "an instance id"
	}
	return "an instance id and a URL"
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
	Args:  instanceIDArgs(2),
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
	Args:  instanceIDArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.InstanceStop(rt.client, rt.base, rt.token, args)
		})
	},
}

var instanceRestartCmd = &cobra.Command{
	Use:   "restart <id>",
	Short: "Soft restart the browser process for an instance",
	Args:  instanceIDArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.InstanceRestart(rt.client, rt.base, rt.token, args)
		})
	},
}

var instanceLogsCmd = &cobra.Command{
	Use:   "logs <id>",
	Short: "Get instance logs",
	Args:  instanceIDArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.InstanceLogs(rt.client, rt.base, rt.token, args)
		})
	},
}
