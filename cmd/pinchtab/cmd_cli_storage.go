package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage browser storage",
	Long:  "Commands for reading and writing localStorage and sessionStorage for the active tab's origin.",
}

var storageGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get storage items",
	Long:  "Read localStorage or sessionStorage items for the active tab. Use --type to select local|session (default: both). Use --key to fetch a single item.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StorageGet(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var storageSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a storage item",
	Long:  "Write a single key/value pair to localStorage or sessionStorage. Use --type local|session (default: local).",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StorageSet(rt.client, rt.base, rt.token, cmd, args[0], args[1])
		})
	},
}

var storageDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a specific storage key",
	Long:  "Remove a single key from localStorage or sessionStorage. Use --key and --type local|session.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StorageDelete(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var storageClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear storage",
	Long:  "Clear localStorage, sessionStorage, or both. Use --type local|session to clear one, or --all to clear both in a single call.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.StorageClear(rt.client, rt.base, rt.token, cmd)
		})
	},
}

func init() {
	storageCmd.AddCommand(storageGetCmd, storageSetCmd, storageDeleteCmd, storageClearCmd)

	addTabFlag(storageGetCmd, storageSetCmd, storageDeleteCmd, storageClearCmd)

	storageGetCmd.Flags().String("type", "", "Storage type: local, session (default: both)")
	storageGetCmd.Flags().String("key", "", "Specific key to retrieve")

	storageSetCmd.Flags().String("type", "local", "Storage type: local or session")

	storageDeleteCmd.Flags().String("type", "local", "Storage type: local or session")
	storageDeleteCmd.Flags().String("key", "", "Key to remove (omit to clear entire store)")

	storageClearCmd.Flags().String("type", "local", "Storage type: local or session")
	storageClearCmd.Flags().Bool("all", false, "Clear both localStorage and sessionStorage")
}
