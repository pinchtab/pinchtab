package main

import (
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func init() {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "Agent session management",
	}

	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show current agent session details",
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				apiclient.DoGet(rt.client, rt.base, rt.token, "/sessions/me", nil)
			})
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all agent sessions",
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				apiclient.DoGet(rt.client, rt.base, rt.token, "/sessions", nil)
			})
		},
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new agent session",
		Run: func(cmd *cobra.Command, args []string) {
			agentID, _ := cmd.Flags().GetString("agent-id")
			label, _ := cmd.Flags().GetString("label")
			if agentID == "" {
				fmt.Fprintln(os.Stderr, "Error: --agent-id is required")
				os.Exit(1)
			}
			body := map[string]any{"agentId": agentID}
			if label != "" {
				body["label"] = label
			}
			runCLI(func(rt cliRuntime) {
				apiclient.DoPost(rt.client, rt.base, rt.token, "/sessions", body)
			})
		},
	}
	createCmd.Flags().String("agent-id", "", "Agent ID to associate with the session (required)")
	createCmd.Flags().String("label", "", "Optional human-readable label")

	revokeCmd := &cobra.Command{
		Use:   "revoke <session-id>",
		Short: "Revoke an agent session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				apiclient.DoPost(rt.client, rt.base, rt.token, "/sessions/"+args[0]+"/revoke", nil)
			})
		},
	}

	sessionCmd.AddCommand(infoCmd, listCmd, createCmd, revokeCmd)
	rootCmd.AddCommand(sessionCmd)
}
