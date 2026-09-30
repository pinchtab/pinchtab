package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetHeaders sets extra HTTP headers via the HTTP API.
func SetHeaders(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "ERROR: set headers requires a JSON object argument")
		os.Exit(2)
	}

	var headers map[string]string
	if err := json.Unmarshal([]byte(args[0]), &headers); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid JSON: %v\n", err)
		os.Exit(2)
	}

	body := map[string]any{
		"headers": headers,
	}

	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/headers")

	result := apiclient.DoPostQuiet(client, base, token, path, body)
	if result == nil {
		fmt.Fprintln(os.Stderr, "ERROR: set headers failed")
		os.Exit(2)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println("applied")
	}
}
