package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage browser session state",
	Long:  "Commands for saving, loading, and managing persistent browser state (cookies, localStorage, sessionStorage).",
}

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved state files",
	Long:  "List all saved state files in the state directory.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StateList(rt.client, rt.base, rt.token)
		})
	},
}

var stateSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save current browser state",
	Long:  "Capture cookies, localStorage, and sessionStorage for the active tab and persist to disk. Requires security.allowStateExport=true.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StateSave(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var stateLoadCmd = &cobra.Command{
	Use:   "load",
	Short: "Load and restore a saved state",
	Long:  "Restore cookies and storage from a previously saved state file. Supports exact name or prefix matching (most recent match is used). Requires security.allowStateExport=true.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StateLoad(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var stateShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show state file details",
	Long:  "Display the full contents of a saved state file including cookies and storage. Requires security.allowStateExport=true.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StateShow(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var stateDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a saved state file",
	Long:  "Remove a named state file from the state directory. Requires security.allowStateExport=true.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StateDelete(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var stateCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove old state files",
	Long:  "Delete state files older than a given number of hours (default: 24). Requires security.allowStateExport=true.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StateClean(rt.client, rt.base, rt.token, cmd)
		})
	},
}

func init() {
	stateCmd.AddCommand(stateListCmd, stateSaveCmd, stateLoadCmd, stateShowCmd, stateDeleteCmd, stateCleanCmd)

	// save flags
	stateSaveCmd.Flags().String("name", "", "Name for the saved state (auto-generated if omitted)")
	stateSaveCmd.Flags().Bool("encrypt", false, "Encrypt the state file (requires security.stateEncryptionKey in config)")
	addTabFlag(stateSaveCmd)

	// load flags
	stateLoadCmd.Flags().String("name", "", "Exact name or prefix of the state file to load")
	_ = stateLoadCmd.MarkFlagRequired("name")
	addTabFlag(stateLoadCmd)

	// show flags
	stateShowCmd.Flags().String("name", "", "Name of the state file to inspect")
	_ = stateShowCmd.MarkFlagRequired("name")

	// delete flags
	stateDeleteCmd.Flags().String("name", "", "Name of the state file to delete")
	_ = stateDeleteCmd.MarkFlagRequired("name")

	// clean flags
	stateCleanCmd.Flags().Int("older-than", 24, "Remove files older than this many hours")
}
