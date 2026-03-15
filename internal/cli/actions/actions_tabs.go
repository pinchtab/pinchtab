package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

// TabList lists all open tabs.
func TabList(client *http.Client, base, token string) {
	apiclient.DoGet(client, base, token, "/tabs", nil)
}

// TabNew opens a new tab (exported for cobra subcommand).
func TabNew(client *http.Client, base, token string, body map[string]any) {
	// Check if any instances are running
	instances := getInstances(client, base, token)
	if len(instances) == 0 {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.WarningStyle, "No instances running, launching default..."))
		launchInstance(client, base, token, "default")
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.SuccessStyle, "Instance launched"))
	}
	apiclient.DoPost(client, base, token, "/tab", body)
}

// TabClose closes a tab by index or ID.
func TabClose(client *http.Client, base, token string, ref string) {
	tabID := resolveTabRef(client, base, token, ref)
	if tabID == "" {
		return
	}
	apiclient.DoPost(client, base, token, "/tab", map[string]any{
		"action": "close",
		"tabId":  tabID,
	})
}

// TabFocus switches to a tab by index or ID, making it the active tab
// for subsequent commands.
func TabFocus(client *http.Client, base, token string, ref string) {
	tabID := resolveTabRef(client, base, token, ref)
	if tabID == "" {
		return
	}
	apiclient.DoPost(client, base, token, "/tab", map[string]any{
		"action": "focus",
		"tabId":  tabID,
	})
}

// resolveTabRef resolves a tab reference that can be either a 1-based index
// (e.g., "1", "2") or a tab ID (e.g., "A1B2C3D4...").
// Returns the resolved tab ID, or empty string on error (already printed).
func resolveTabRef(client *http.Client, base, token string, ref string) string {
	n, err := strconv.Atoi(ref)
	if err != nil {
		// Not a number — treat as tab ID directly
		return ref
	}

	// Numeric — resolve index to tab ID via /tabs
	body := apiclient.DoGetRaw(client, base, token, "/tabs", nil)
	var resp struct {
		Tabs []struct {
			ID string `json:"id"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("Failed to parse tabs: %v", err)))
		return ""
	}
	if n < 1 || n > len(resp.Tabs) {
		fmt.Fprintln(os.Stderr, cli.StyleStderr(cli.ErrorStyle, fmt.Sprintf("Tab index %d out of range (1-%d)", n, len(resp.Tabs))))
		return ""
	}
	return resp.Tabs[n-1].ID
}
