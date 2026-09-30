package actions

import (
	"fmt"
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
func NetworkRoute(conn apiclient.Conn, cmd *cobra.Command, pattern string) {
	abort, _ := cmd.Flags().GetBool("abort")
	body, _ := cmd.Flags().GetString("body")
	resourceType, _ := cmd.Flags().GetString("resource-type")
	contentType, _ := cmd.Flags().GetString("content-type")
	status, _ := cmd.Flags().GetInt("status")
	method, _ := cmd.Flags().GetString("method")

	if abort && body != "" {
		usageErr("Error: --abort and --body are mutually exclusive")
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

	path := networkRoutePath(cmd)
	result := requireMap(conn.PostQuiet(path, req), "Failed to install route")

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		printIndented(result)
		return
	}
	fmt.Printf("route installed: %s (%s)\n", pattern, req["action"])
}

// NetworkUnroute removes one or all rules from the active tab. Empty pattern
// removes all rules.
func NetworkUnroute(conn apiclient.Conn, cmd *cobra.Command, pattern string) {
	params := url.Values{}
	if pattern != "" {
		params.Set("pattern", pattern)
	}
	path := networkRoutePath(cmd)
	result := requireMap(conn.Delete(path, params), "Failed to remove route(s)")

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

func NetworkRules(conn apiclient.Conn, cmd *cobra.Command) {
	path := networkRoutePath(cmd)
	result := requireMap(conn.Get(path, nil), "Failed to list routes")

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		printIndented(result)
		return
	}
	rules, _ := result["rules"].([]any)
	if len(rules) == 0 {
		fmt.Println("no interception rules: this tab mocks and blocks nothing")
		return
	}
	for _, entry := range rules {
		rule, _ := entry.(map[string]any)
		fmt.Println(describeRouteRule(rule))
	}
}

func describeRouteRule(rule map[string]any) string {
	line := fmt.Sprintf("%v (%v)", rule["pattern"], rule["action"])
	for _, key := range []string{"method", "resourceType", "status", "contentType"} {
		if v, ok := rule[key]; ok && v != nil && v != "" && v != float64(0) {
			line += fmt.Sprintf(" %s=%v", key, v)
		}
	}
	if body, ok := rule["body"].(string); ok && body != "" {
		line += fmt.Sprintf(" body=%d bytes", len(body))
	}
	return line
}

func networkRoutePath(cmd *cobra.Command) string {
	tab, _ := cmd.Flags().GetString("tab")
	return apiclient.TabPath(tab, "network/route")
}
