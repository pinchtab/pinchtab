package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/urls"
	"github.com/spf13/cobra"
)

var quickCmd = &cobra.Command{
	Use:   "quick <url>",
	Short: "Navigate + analyze page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		args[0] = urls.Normalize(args[0])
		runCLI(func(rt cliRuntime) {
			browseractions.Quick(rt.client, rt.base, rt.token, args)
		})
	},
}

var navCmd = &cobra.Command{
	Use:     "nav <url>",
	Aliases: []string{"goto", "navigate", "open"},
	Short:   "Navigate to URL",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		url := urls.Normalize(args[0])
		runCLI(func(rt cliRuntime) {
			tabID := browseractions.Navigate(rt.client, rt.base, rt.token, url, cmd)
			WriteTabStateFile(tabID)
		})
	},
}

var backCmd = &cobra.Command{
	Use:   "back",
	Short: "Go back in browser history",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Back(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var forwardCmd = &cobra.Command{
	Use:   "forward",
	Short: "Go forward in browser history",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Forward(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload current page",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Reload(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var tabsCmd = &cobra.Command{
	Use:     "tab [id]",
	Aliases: []string{"tabs"},
	Short:   "List tabs, or focus a tab by ID",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			if len(args) == 0 {
				browseractions.TabList(rt.client, rt.base, rt.token, cmd)
				return
			}
			browseractions.TabFocus(rt.client, rt.base, rt.token, args[0], cmd)
		})
	},
}

var tabNewCmd = &cobra.Command{
	Use:   "new [url]",
	Short: "Open a new tab",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			body := map[string]any{"action": "new"}
			if len(args) > 0 {
				body["url"] = urls.Normalize(args[0])
			}
			browseractions.TabNew(rt.client, rt.base, rt.token, body, cmd)
		})
	},
}

var tabCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Close a tab by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.TabClose(rt.client, rt.base, rt.token, args[0], cmd)
		})
	},
}

var tabHandoffCmd = &cobra.Command{
	Use:   "handoff [id]",
	Short: "Pause tab automation for human handoff",
	Long:  "Mark a tab as paused_handoff so action routes block until resumed or timeout expires. Defaults to the current tab (PINCHTAB_TAB or state file).",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		reason, _ := cmd.Flags().GetString("reason")
		timeoutMS, _ := cmd.Flags().GetInt("timeout-ms")
		tabID := resolveTabArg(args)
		runCLI(func(rt cliRuntime) {
			browseractions.TabHandoff(rt.client, rt.base, rt.token, tabID, reason, timeoutMS, cmd)
		})
	},
}

var tabResumeCmd = &cobra.Command{
	Use:   "resume [id]",
	Short: "Resume a paused_handoff tab",
	Long:  "Resume automation on a paused tab. Defaults to the current tab (PINCHTAB_TAB or state file).",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		status, _ := cmd.Flags().GetString("status")
		tabID := resolveTabArg(args)
		runCLI(func(rt cliRuntime) {
			browseractions.TabResume(rt.client, rt.base, rt.token, tabID, status, cmd)
		})
	},
}

var tabHandoffStatusCmd = &cobra.Command{
	Use:   "handoff-status [id]",
	Short: "Show handoff status for a tab",
	Long:  "Show handoff status. Defaults to the current tab (PINCHTAB_TAB or state file).",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		tabID := resolveTabArg(args)
		runCLI(func(rt cliRuntime) {
			browseractions.TabHandoffStatus(rt.client, rt.base, rt.token, tabID, cmd)
		})
	},
}
