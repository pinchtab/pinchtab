package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetViewport sets the browser viewport dimensions via the HTTP API.
func SetViewport(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "ERROR: set viewport requires <width> <height> arguments")
		os.Exit(2)
	}

	var width, height int
	if _, err := fmt.Sscanf(args[0], "%d", &width); err != nil || width <= 0 {
		fmt.Fprintf(os.Stderr, "ERROR: invalid width %q: must be a positive integer\n", args[0])
		os.Exit(2)
	}
	if _, err := fmt.Sscanf(args[1], "%d", &height); err != nil || height <= 0 {
		fmt.Fprintf(os.Stderr, "ERROR: invalid height %q: must be a positive integer\n", args[1])
		os.Exit(2)
	}

	body := map[string]any{
		"width":  width,
		"height": height,
	}

	if dpr, _ := cmd.Flags().GetFloat64("dpr"); dpr > 0 {
		body["deviceScaleFactor"] = dpr
	}
	if mobile, _ := cmd.Flags().GetBool("mobile"); mobile {
		body["mobile"] = true
	}

	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/viewport")

	result := apiclient.DoPostQuiet(client, base, token, path, body)
	if result == nil {
		fmt.Fprintln(os.Stderr, "ERROR: set viewport failed")
		os.Exit(2)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("%dx%d\n", width, height)
	}
}
