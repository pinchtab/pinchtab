package main

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"

	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage browser storage",
	Long:  "Commands for reading and writing localStorage and sessionStorage for the active tab's origin.",
}

var storageGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get storage items",
	Long:  "Read localStorage or sessionStorage items for the active tab. Use --type to select local|session (default: both). Pass a key (or --key) to fetch a single item; omit it to list the store.",
	Args:  optionalOperand("key"),
	Run: func(cmd *cobra.Command, args []string) {
		key := operandOrFlag(cmd, args, "key")
		runCLI(func(rt apiclient.Conn) {
			browseractions.StorageGet(rt, cmd, key)
		})
	},
}

var storageSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a storage item",
	Long:  "Write a single key/value pair to localStorage or sessionStorage. Use --type local|session (default: local).",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.StorageSet(rt, cmd, args[0], args[1])
		})
	},
}

var storageDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a specific storage key",
	Long:  "Remove a single key from localStorage or sessionStorage. Pass the key (or --key) and --type local|session. A key is required: to wipe a whole store use \"pinchtab storage clear\".",
	Args:  requiredOperand("key", `to wipe the whole store use "pinchtab storage clear"`),
	Run: func(cmd *cobra.Command, args []string) {
		key := operandOrFlag(cmd, args, "key")
		runCLI(func(rt apiclient.Conn) {
			browseractions.StorageDelete(rt, cmd, key)
		})
	},
}

var storageClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear storage",
	Long:  "Clear localStorage, sessionStorage, or both. Use --type local|session to clear one, or --all to clear both in a single call.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt apiclient.Conn) {
			browseractions.StorageClear(rt, cmd)
		})
	},
}

func operandOrFlag(cmd *cobra.Command, args []string, flag string) string {
	if len(args) > 0 {
		return args[0]
	}
	value, _ := cmd.Flags().GetString(flag)
	return value
}

func optionalOperand(flag string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
			return err
		}
		if len(args) == 0 || !cmd.Flags().Changed(flag) {
			return nil
		}
		value, _ := cmd.Flags().GetString(flag)
		return fmt.Errorf("%s got the %s twice, %q as the <%s> argument and %q as --%s; pass it once, as the argument or as --%s",
			cmd.CommandPath(), flag, args[0], flag, value, flag, flag)
	}
}

func requiredOperand(flag, remedy string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := optionalOperand(flag)(cmd, args); err != nil {
			return err
		}
		if operandOrFlag(cmd, args, flag) != "" {
			return nil
		}
		return fmt.Errorf("%s needs a %s: %s <%s>; %s", cmd.CommandPath(), flag, cmd.CommandPath(), flag, remedy)
	}
}

func init() {
	storageCmd.AddCommand(storageGetCmd, storageSetCmd, storageDeleteCmd, storageClearCmd)

	addTabFlag(storageGetCmd, storageSetCmd, storageDeleteCmd, storageClearCmd)

	storageGetCmd.Flags().String("type", "", "Storage type: local, session (default: both)")
	storageGetCmd.Flags().String("key", "", "Specific key to retrieve (same as the <key> argument)")

	storageSetCmd.Flags().String("type", "local", "Storage type: local or session")

	storageDeleteCmd.Flags().String("type", "local", "Storage type: local or session")
	storageDeleteCmd.Flags().String("key", "", "Key to remove (same as the <key> argument)")

	storageClearCmd.Flags().String("type", "local", "Storage type: local or session")
	storageClearCmd.Flags().Bool("all", false, "Clear both localStorage and sessionStorage")
}
