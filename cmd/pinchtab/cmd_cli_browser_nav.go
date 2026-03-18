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
			browseractions.Navigate(rt.client, rt.base, rt.token, url, cmd)
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
				browseractions.TabList(rt.client, rt.base, rt.token)
				return
			}
			browseractions.TabFocus(rt.client, rt.base, rt.token, args[0])
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
			browseractions.TabNew(rt.client, rt.base, rt.token, body)
		})
	},
}

var tabCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Close a tab by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.TabClose(rt.client, rt.base, rt.token, args[0])
		})
	},
}
