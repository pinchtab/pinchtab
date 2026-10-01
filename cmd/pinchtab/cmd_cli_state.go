package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

const stateListRemedy = `run "pinchtab state list" to see saved states`

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Show current browser state or manage saved state",
	Long:  "Show current browser state for the current tab, or save, load, and manage persistent browser state (cookies, localStorage, sessionStorage).",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.StateCurrent(rt, cmd)
		})
	},
}

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved state files",
	Long:  "List all saved state files in the state directory.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.StateList(rt)
		})
	},
}

var stateSaveCmd = &cobra.Command{
	Use:   "save [name]",
	Short: "Save current browser state",
	Long:  "Capture cookies, localStorage, and sessionStorage for the active tab and persist to disk under the given name (or --name); omit it to auto-generate one. Requires security.allowStateExport=true.",
	Args:  optionalOperand("name"),
	Run: func(cmd *cobra.Command, args []string) {
		name := operandOrFlag(cmd, args, "name")
		runCLI(func(rt apiclient.Conn) {
			browseractions.StateSave(rt, cmd, name)
		})
	},
}

var stateLoadCmd = &cobra.Command{
	Use:   "load <name>",
	Short: "Load and restore a saved state",
	Long:  "Restore cookies and storage from a previously saved state file, named as the argument (or --name). Supports exact name or prefix matching (most recent match is used). Requires security.allowStateExport=true.",
	Args:  requiredOperand("name", stateListRemedy),
	Run: func(cmd *cobra.Command, args []string) {
		name := operandOrFlag(cmd, args, "name")
		runCLI(func(rt apiclient.Conn) {
			browseractions.StateLoad(rt, cmd, name)
		})
	},
}

var stateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show state file details",
	Long:  "Display the full contents of a saved state file, named as the argument (or --name), including cookies and storage. Requires security.allowStateExport=true.",
	Args:  requiredOperand("name", stateListRemedy),
	Run: func(cmd *cobra.Command, args []string) {
		name := operandOrFlag(cmd, args, "name")
		runCLI(func(rt apiclient.Conn) {
			browseractions.StateShow(rt, name)
		})
	},
}

var stateDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a saved state file",
	Long:  "Remove a state file, named as the argument (or --name), from the state directory. Requires security.allowStateExport=true.",
	Args:  requiredOperand("name", stateListRemedy),
	Run: func(cmd *cobra.Command, args []string) {
		name := operandOrFlag(cmd, args, "name")
		runCLI(func(rt apiclient.Conn) {
			browseractions.StateDelete(rt, name)
		})
	},
}

var stateCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove old state files",
	Long:  "Delete state files older than a given number of hours (default: 24). Requires security.allowStateExport=true.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.StateClean(rt, cmd)
		})
	},
}

func init() {
	stateCmd.AddCommand(stateListCmd, stateSaveCmd, stateLoadCmd, stateShowCmd, stateDeleteCmd, stateCleanCmd)
	addTabFlag(stateCmd)

	stateSaveCmd.Flags().String("name", "", "Name for the saved state, same as the [name] argument (auto-generated if omitted)")
	stateSaveCmd.Flags().Bool("encrypt", false, "Encrypt the state file (requires security.stateEncryptionKey in config)")
	addTabFlag(stateSaveCmd)

	stateLoadCmd.Flags().String("name", "", "Exact name or prefix of the state file to load (same as the <name> argument)")
	addTabFlag(stateLoadCmd)

	stateShowCmd.Flags().String("name", "", "Name of the state file to inspect (same as the <name> argument)")

	stateDeleteCmd.Flags().String("name", "", "Name of the state file to delete (same as the <name> argument)")

	stateCleanCmd.Flags().Int("older-than", 24, "Remove files older than this many hours")
}
