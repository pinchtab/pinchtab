package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetOffline enables or disables network offline emulation via the HTTP API.
func SetOffline(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "ERROR: set offline requires <true|false> argument")
		os.Exit(2)
	}

	offline, err := strconv.ParseBool(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid offline value %q: must be true or false\n", args[0])
		os.Exit(2)
	}

	body := map[string]any{
		"offline": offline,
	}

	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/offline")

	result := apiclient.DoPostQuiet(client, base, token, path, body)
	if result == nil {
		fmt.Fprintln(os.Stderr, "ERROR: set offline failed")
		os.Exit(2)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		if offline {
			fmt.Println("offline")
		} else {
			fmt.Println("online")
		}
	}
}
