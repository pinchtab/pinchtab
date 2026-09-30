package actions

import (
	"net/http"

	"github.com/spf13/cobra"
)

// SetCredentials sets HTTP auth credentials via the HTTP API.
func SetCredentials(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	requireArgs(args, 2, "set credentials requires <username> <password> arguments")
	postEmulation(client, base, token, cmd, "credentials", map[string]any{"username": args[0], "password": args[1]}, responseStatus)
}
