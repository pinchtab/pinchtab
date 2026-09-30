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
			browseractions.NetworkRoute(rt, cmd, args[0])
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
			browseractions.NetworkUnroute(rt, cmd, pattern)
		})
	},
}

var networkRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List the interception rules installed on the active tab",
	Long: `List every interception rule on the active tab, one per line as pattern (action):
continue passes the request through, abort blocks it, fulfill answers it with the
rule's body instead of the real server. Rules survive navigation, so a tab you did not
set up can be mocking or blocking traffic; an empty list means it does neither.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.NetworkRules(rt, cmd)
		})
	},
}

func configureNetworkRouteFlags() {
	networkRouteCmd.Flags().Bool("abort", false, "Block matching requests instead of letting them through")
	networkRouteCmd.Flags().String("body", "", "Fulfill matching requests with this JSON body (mutually exclusive with --abort)")
	networkRouteCmd.Flags().String("resource-type", "", "Limit to a CDP resource category (e.g. script, image, xhr, fetch)")
	networkRouteCmd.Flags().String("content-type", "", "(With --body) Response Content-Type (default application/json)")
	networkRouteCmd.Flags().Int("status", 0, "(With --body) Response status code (default 200)")
	networkRouteCmd.Flags().String("method", "", "Limit to an HTTP method (GET, POST, ...). Fulfill rules without --method skip OPTIONS preflights to avoid breaking CORS.")
	addTabFlag(networkRouteCmd)
	addTabFlag(networkUnrouteCmd)
	addTabFlag(networkRulesCmd)
	addJSONFlag(networkRouteCmd)
	addJSONFlag(networkUnrouteCmd)
	addJSONFlag(networkRulesCmd)
}
