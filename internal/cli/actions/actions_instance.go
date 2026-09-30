package actions

import (
	"fmt"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func InstanceStart(conn apiclient.Conn, cmd *cobra.Command) {
	body := map[string]any{}
	if v, _ := cmd.Flags().GetString("profile"); v != "" {
		body["profileId"] = v
	}
	if v, _ := cmd.Flags().GetString("mode"); v != "" {
		body["mode"] = v
	}
	if v, _ := cmd.Flags().GetString("port"); v != "" {
		body["port"] = v
	}
	if exts, _ := cmd.Flags().GetStringArray("extension"); len(exts) > 0 {
		body["extensionPaths"] = exts
	}
	if domains, _ := cmd.Flags().GetStringArray("allow-domain"); len(domains) > 0 {
		body["securityPolicy"] = map[string]any{
			"allowedDomains": domains,
		}
	}
	if v, _ := cmd.Flags().GetString("browser"); v != "" {
		body["browser"] = v
	}
	if fallbacks, _ := cmd.Flags().GetStringArray("browser-fallback"); len(fallbacks) > 0 {
		body["fallbackTargets"] = fallbacks
	}
	conn.Post("/instances/start", body)
}

func InstanceNavigate(conn apiclient.Conn, args []string) {
	if len(args) < 2 {
		cli.Fatal("Usage: pinchtab instance navigate <instance-id> <url>")
	}

	instID := args[0]
	targetURL := args[1]

	conn.Post(fmt.Sprintf("/instances/%s/tabs/open", instID), map[string]any{
		"url": targetURL,
	})
}

func InstanceLogs(conn apiclient.Conn, args []string) {
	instID := args[0]
	logs := conn.GetRaw(fmt.Sprintf("/instances/%s/logs", instID), nil)
	fmt.Println(string(logs))
}

func InstanceStop(conn apiclient.Conn, args []string) {
	instID := args[0]
	conn.Post(fmt.Sprintf("/instances/%s/stop", instID), nil)
}

func InstanceRestart(conn apiclient.Conn, args []string) {
	instID := args[0]
	conn.Post(fmt.Sprintf("/instances/%s/restart", instID), nil)
}
