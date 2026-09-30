package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage browser cache",
	Long:  "Commands for managing the browser's HTTP disk cache.",
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear browser HTTP cache",
	Long:  "Clear the browser's HTTP disk cache. This affects all origins and ensures fresh resources are fetched on subsequent navigations.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.CacheClear(rt, cmd)
		})
	},
}

var cacheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check cache status",
	Long:  "Check if the browser cache can be cleared.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.CacheStatus(rt, cmd)
		})
	},
}

func init() {
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheStatusCmd)
}

func configureCacheFlags() {
	addJSONFlag(cacheClearCmd)
	addJSONFlag(cacheStatusCmd)
}
