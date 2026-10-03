package actions

import (
	"encoding/json"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// TabList lists all open tabs.
func TabList(conn apiclient.Conn, cmd *cobra.Command, targetTabID string) {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Get("/tabs", nil)
		return
	}

	body := conn.GetRaw("/tabs", nil)
	var resp struct {
		Tabs []struct {
			ID    string `json:"id"`
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		fmt.Println(string(body))
		return
	}
	for _, tab := range resp.Tabs {
		marker := ""
		if targetTabID != "" && tab.ID == targetTabID {
			marker = "*"
		}
		fmt.Printf("%s%s\t%s\t%s\n", marker, tab.ID, tab.URL, tab.Title)
	}
}

// TabClose closes a tab by ID. When that tab was the CLI's current one it returns
// the tab the server lists first, which is where a read without a tab id lands.
func TabClose(conn apiclient.Conn, tabID string, wasCurrent bool, cmd *cobra.Command) string {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	body := map[string]any{"tabId": tabID}
	if !wasCurrent {
		if jsonOutput {
			conn.Post("/close", body)
			return ""
		}
		conn.PostQuiet("/close", body)
		output.Success()
		return ""
	}

	result := conn.PostQuiet("/close", body)
	successor := firstListedTabID(conn)
	if jsonOutput {
		if result == nil {
			result = map[string]any{}
		}
		result["currentTabId"] = successor
		output.JSON(result)
		return successor
	}
	output.Success()
	if successor == "" {
		output.Hint("no tab is current: pass a tab id, or run pinchtab nav <url> to open one")
		return ""
	}
	fmt.Printf("current tab: %s\n", successor)
	return successor
}

func firstListedTabID(conn apiclient.Conn) string {
	ids, err := listTabIDs(conn)
	if err != nil || len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// TabFocus switches to a tab by ID, making it the active tab
// for subsequent commands.
func TabFocus(conn apiclient.Conn, tabID string, cmd *cobra.Command) string {
	resolvedTabID, err := resolveTabReference(conn, tabID)
	if err != nil {
		cli.Fatal("%v", err)
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		result := conn.Post("/tab", map[string]any{
			"action": "focus",
			"tabId":  resolvedTabID,
		})
		if tid, ok := result["tabId"].(string); ok && tid != "" {
			return tid
		}
		return resolvedTabID
	}

	result := conn.PostQuiet("/tab", map[string]any{
		"action": "focus",
		"tabId":  resolvedTabID,
	})
	if tid, ok := result["tabId"].(string); ok && tid != "" {
		output.Value(tid)
		return tid
	}
	output.Value(resolvedTabID)
	return resolvedTabID
}

// TabHandoff pauses automation on a tab for manual operator intervention.
func TabHandoff(conn apiclient.Conn, tabID, reason string, timeoutMS int, cmd *cobra.Command) {
	body := map[string]any{}
	if reason != "" {
		body["reason"] = reason
	}
	if timeoutMS > 0 {
		body["timeoutMs"] = timeoutMS
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Post(apiclient.TabPath(tabID, "handoff"), body)
		return
	}
	conn.PostQuiet(apiclient.TabPath(tabID, "handoff"), body)
	output.Value("paused")
	if reason != "" {
		output.Hint(fmt.Sprintf("reason: %s", reason))
	}
}

// TabResume resumes automation on a tab after manual intervention.
func TabResume(conn apiclient.Conn, tabID, status string, cmd *cobra.Command) {
	body := map[string]any{}
	if status != "" {
		body["status"] = status
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Post(apiclient.TabPath(tabID, "resume"), body)
		return
	}
	conn.PostQuiet(apiclient.TabPath(tabID, "resume"), body)
	output.Value("resumed")
}

// TabHandoffStatus shows whether a tab is in paused_handoff or active state.
func TabHandoffStatus(conn apiclient.Conn, tabID string, cmd *cobra.Command) {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Get(apiclient.TabPath(tabID, "handoff"), nil)
		return
	}
	body := conn.GetRaw(apiclient.TabPath(tabID, "handoff"), nil)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println(string(body))
		return
	}
	status, _ := result["status"].(string)
	output.Value(status)
	if reason, ok := result["reason"].(string); ok && reason != "" {
		output.Hint(fmt.Sprintf("reason: %s", reason))
	}
}
