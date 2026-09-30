package actions

import (
	"fmt"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetGeolocation sets the browser geolocation via the HTTP API.
func SetGeolocation(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	requireArgs(args, 2, "set geo requires <latitude> <longitude> arguments")
	latitude := parseFloatArg("latitude", args[0])
	longitude := parseFloatArg("longitude", args[1])

	body := map[string]any{"latitude": latitude, "longitude": longitude}
	if accuracy, _ := cmd.Flags().GetFloat64("accuracy"); accuracy > 0 {
		body["accuracy"] = accuracy
	}

	postEmulation(conn, cmd, "geolocation", body, func(map[string]any) string {
		return fmt.Sprintf("%.6f,%.6f", latitude, longitude)
	})
}

func parseFloatArg(name, raw string) float64 {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		usageErr("ERROR: invalid %s %q: must be a number", name, raw)
	}
	return value
}
