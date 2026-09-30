package actions

import (
	"encoding/json"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// SetHeaders sets extra HTTP headers via the HTTP API.
func SetHeaders(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	requireArgs(args, 1, "set headers requires a JSON object argument")
	var headers map[string]string
	if err := json.Unmarshal([]byte(args[0]), &headers); err != nil {
		usageErr("ERROR: invalid JSON: %v", err)
	}

	postEmulation(conn, cmd, "headers", map[string]any{"headers": headers}, func(map[string]any) string {
		return "applied"
	})
}
