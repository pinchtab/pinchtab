package main

import (
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
				apiclient.DoGet(rt.client, rt.base, rt.token, "/api/sessions/me", nil)
			})
		},
	}

	sessionCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(sessionCmd)
}
