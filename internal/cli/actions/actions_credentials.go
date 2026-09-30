package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetCredentials sets HTTP auth credentials via the HTTP API.
func SetCredentials(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "ERROR: set credentials requires <username> <password> arguments")
		os.Exit(2)
	}

	body := map[string]any{
		"username": args[0],
		"password": args[1],
	}

	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/credentials")

	result := apiclient.DoPostQuiet(client, base, token, path, body)
	if result == nil {
		fmt.Fprintln(os.Stderr, "ERROR: set credentials failed")
		os.Exit(2)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		status, _ := result["status"].(string)
		fmt.Println(status)
	}
}
