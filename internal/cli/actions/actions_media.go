package actions

import (
	"net/http"

	"github.com/spf13/cobra"
)

// SetMedia emulates a CSS media feature via the HTTP API.
func SetMedia(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	requireArgs(args, 2, "set media requires <feature> <value> arguments")
	postEmulation(client, base, token, cmd, "media", map[string]any{"feature": args[0], "value": args[1]}, responseStatus)
}
