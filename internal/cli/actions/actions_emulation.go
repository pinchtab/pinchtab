package actions

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// SetViewport sets the browser viewport dimensions via the HTTP API.
func SetViewport(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	requireArgs(args, 2, "set viewport requires <width> <height> arguments")
	width := parsePositiveIntArg("width", args[0])
	height := parsePositiveIntArg("height", args[1])

	body := map[string]any{"width": width, "height": height}
	if dpr, _ := cmd.Flags().GetFloat64("dpr"); dpr > 0 {
		body["deviceScaleFactor"] = dpr
	}
	if mobile, _ := cmd.Flags().GetBool("mobile"); mobile {
		body["mobile"] = true
	}

	postEmulation(client, base, token, cmd, "viewport", body, func(map[string]any) string {
		return fmt.Sprintf("%dx%d", width, height)
	})
}

func parsePositiveIntArg(name, raw string) int {
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil || value <= 0 {
		usageErr("ERROR: invalid %s %q: must be a positive integer", name, raw)
	}
	return value
}
