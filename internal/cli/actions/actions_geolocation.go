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

// SetGeolocation sets the browser geolocation via the HTTP API.
func SetGeolocation(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "ERROR: set geo requires <latitude> <longitude> arguments")
		os.Exit(2)
	}

	latitude, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid latitude %q: must be a number\n", args[0])
		os.Exit(2)
	}

	longitude, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid longitude %q: must be a number\n", args[1])
		os.Exit(2)
	}

	body := map[string]any{
		"latitude":  latitude,
		"longitude": longitude,
	}

	if accuracy, _ := cmd.Flags().GetFloat64("accuracy"); accuracy > 0 {
		body["accuracy"] = accuracy
	}

	tab, _ := cmd.Flags().GetString("tab")
	path := apiclient.TabPath(tab, "emulation/geolocation")

	result := apiclient.DoPostQuiet(client, base, token, path, body)
	if result == nil {
		fmt.Fprintln(os.Stderr, "ERROR: set geo failed")
		os.Exit(2)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("%.6f,%.6f\n", latitude, longitude)
	}
}
