package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetMedia emulates a CSS media feature via the HTTP API.
func SetMedia(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "ERROR: set media requires <feature> <value> arguments")
		os.Exit(2)
	}

	body := map[string]any{
		"feature": args[0],
		"value":   args[1],
	}

	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/media")

	result := apiclient.DoPostQuiet(client, base, token, path, body)
	if result == nil {
		fmt.Fprintln(os.Stderr, "ERROR: set media failed")
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
