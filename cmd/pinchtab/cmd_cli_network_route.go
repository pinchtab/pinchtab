package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var networkRouteCmd = &cobra.Command{
	Use:   "route <url>",
	Short: "Intercept matching requests on the active tab",
	Long: `Install a network interception rule on the active tab. The pattern is matched
against request URLs as a substring (no wildcards) or as a glob ('*', '?').

  pinchtab network route '*.png' --abort           # block matching requests
  pinchtab network route 'api/users' --body '{}'   # fulfill with JSON body
  pinchtab network route 'tracker.io'              # pass-through (no-op rule)

Use 'pinchtab network unroute' to remove a rule.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.NetworkRoute(rt.client, rt.base, rt.token, cmd, args[0])
		})
	},
}

var networkRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List interception rules on the active tab",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.NetworkRules(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var networkUnrouteCmd = &cobra.Command{
	Use:   "unroute [url]",
	Short: "Remove an interception rule (or all rules if no pattern given)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pattern := ""
		if len(args) > 0 {
			pattern = args[0]
		}
		runCLI(func(rt cliRuntime) {
			browseractions.NetworkUnroute(rt.client, rt.base, rt.token, cmd, pattern)
		})
	},
}
