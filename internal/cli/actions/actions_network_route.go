package actions

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// NetworkRoute installs an interception rule on the active tab.
// Behavior depends on flags:
//
//	--abort        : block matching requests
//	--body <json>  : fulfill matching requests with the given JSON body
//
// With neither flag set, the rule is a pass-through (continue).
//
// Optional refinements:
//
//	--resource-type <kind>  : only intercept this CDP resource category
//	--status <code>         : (with --body) override the response status (default 200)
//	--content-type <ct>     : (with --body) override Content-Type (default application/json)
//
// The --tab flag (state file) selects the target tab; when
// empty the bare /network/route endpoint resolves to the active tab.
func NetworkRoute(client *http.Client, base, token string, cmd *cobra.Command, pattern string) {
	abort, _ := cmd.Flags().GetBool("abort")
	body, _ := cmd.Flags().GetString("body")
	resourceType, _ := cmd.Flags().GetString("resource-type")
	contentType, _ := cmd.Flags().GetString("content-type")
	status, _ := cmd.Flags().GetInt("status")
	method, _ := cmd.Flags().GetString("method")

	if abort && body != "" {
		exitErr(1, "Error: --abort and --body are mutually exclusive")
	}

	req := map[string]any{"pattern": pattern}
	switch {
	case abort:
		req["action"] = "abort"
	case body != "":
		req["action"] = "fulfill"
		req["body"] = body
	default:
		req["action"] = "continue"
	}
	if resourceType != "" {
		req["resourceType"] = resourceType
	}
	if contentType != "" {
		req["contentType"] = contentType
	}
	if status != 0 {
		req["status"] = status
	}
	if method != "" {
		req["method"] = method
	}

	path := "/network/route"
	if tab, _ := cmd.Flags().GetString("tab"); tab != "" {
		path = fmt.Sprintf("/tabs/%s/network/route", url.PathEscape(tab))
	}
	result := requireMap(apiclient.DoPostQuiet(client, base, token, path, req), 1, "Failed to install route")

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		printIndented(result)
		return
	}
	fmt.Printf("route installed: %s (%s)\n", pattern, req["action"])
}

// NetworkUnroute removes one or all rules from the active tab. Empty pattern
// removes all rules.
func NetworkUnroute(client *http.Client, base, token string, cmd *cobra.Command, pattern string) {
	params := url.Values{}
	if pattern != "" {
		params.Set("pattern", pattern)
	}
	path := "/network/route"
	if tab, _ := cmd.Flags().GetString("tab"); tab != "" {
		path = fmt.Sprintf("/tabs/%s/network/route", url.PathEscape(tab))
	}
	result := requireMap(apiclient.DoDelete(client, base, token, path, params), 1, "Failed to remove route(s)")

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		printIndented(result)
		return
	}
	if removed, ok := result["removed"].(float64); ok {
		fmt.Printf("removed %d route(s)\n", int(removed))
	} else {
		fmt.Println("routes cleared")
	}
}
