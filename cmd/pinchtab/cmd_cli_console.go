package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "View browser console logs",
	Long:  "View browser console.log/warn/error/info messages from the current or specified tab.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Console(rt, cmd)
		})
	},
}

var errorsCmd = &cobra.Command{
	Use:   "errors",
	Short: "View browser uncaught errors",
	Long:  "View uncaught JavaScript exceptions from the current or specified tab.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.Errors(rt, cmd)
		})
	},
}

func configureConsoleFlags() {
	consoleCmd.Flags().Bool("clear", false, "Clear console logs")
	consoleCmd.Flags().String("limit", "", "Maximum entries to return")
	errorsCmd.Flags().Bool("clear", false, "Clear error logs")
	errorsCmd.Flags().String("limit", "", "Maximum entries to return")
	addJSONFlag(consoleCmd)
	addJSONFlag(errorsCmd)
	addTabFlag(consoleCmd)
	addTabFlag(errorsCmd)
}
