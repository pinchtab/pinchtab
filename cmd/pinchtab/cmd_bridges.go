package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridgeregistry"
	"github.com/spf13/cobra"
)

var bridgesCmd = &cobra.Command{
	Use:     "bridges",
	Short:   "Inspect standalone bridge processes",
	GroupID: "management",
}

var bridgesListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List registered standalone bridges and current liveness",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		prune, _ := cmd.Flags().GetBool("prune")
		states, err := bridgeregistry.List(stateDirForConfig(loadConfig()), prune)
		if err != nil {
			return fmt.Errorf("pinchtab bridges list: %w", err)
		}
		jsonOutput, _ := cmd.Flags().GetBool("json")
		return writeBridges(cmd.OutOrStdout(), states, jsonOutput)
	},
}

func writeBridges(w io.Writer, states []bridgeregistry.State, jsonOutput bool) error {
	if jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(states)
	}
	if len(states) == 0 {
		_, err := fmt.Fprintln(w, "No registered standalone bridges.")
		return err
	}
	for _, state := range states {
		host := strings.Trim(strings.TrimSpace(state.Address), "[]")
		_, err := fmt.Fprintf(w, "%s pid=%d pidStatus=%s reachable=%t bridge=http://%s browser=%q label=%q cdp=%q",
			state.Status, state.PID, state.PIDStatus, state.Reachable, net.JoinHostPort(host, state.Port), state.BrowserType, state.BrowserLabel, state.CDPIdentity)
		if err != nil {
			return err
		}
		if state.Pruned {
			if _, err := io.WriteString(w, " pruned=true"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	addJSONFlag(bridgesListCmd)
	bridgesListCmd.Flags().Bool("prune", false, "Remove records whose original PID is conclusively stale (never signals processes)")
	bridgesCmd.AddCommand(bridgesListCmd)
	rootCmd.AddCommand(bridgesCmd)
}
