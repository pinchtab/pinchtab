package actions

import (
	"fmt"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// CacheClear clears the browser's HTTP disk cache.
func CacheClear(conn apiclient.Conn, cmd *cobra.Command) {
	result := requireMap(conn.PostQuiet("/cache/clear", nil), "ERROR: cache: clear failed")

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		printIndented(result)
	} else {
		fmt.Println("OK")
	}
}

// CacheStatus checks if the browser cache can be cleared.
func CacheStatus(conn apiclient.Conn, cmd *cobra.Command) {
	result := requireBytes(conn.GetRaw("/cache/status", nil), "ERROR: cache: status check failed")
	buf := decodeMap(result, "ERROR: cache")

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		printIndented(buf)
	} else {
		if canClear, ok := buf["canClear"].(bool); ok && canClear {
			fmt.Println("can-clear")
		} else {
			fmt.Println("cache-empty")
		}
	}
}
