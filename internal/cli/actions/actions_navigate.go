package actions

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Back navigates the current (or specified) tab back in history.
func Back(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/back"
	if tabID != "" {
		path = "/tabs/" + tabID + "/back"
	}
	path = appendDismissBannersQuery(path, cmd)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPost(client, base, token, path, nil)
		return
	}

	result := apiclient.DoPostQuiet(client, base, token, path, nil)
	if url, ok := result["url"].(string); ok {
		output.Value(url)
	} else {
		output.Success()
	}

	snap, _ := cmd.Flags().GetBool("snap")
	snapDiff, _ := cmd.Flags().GetBool("snap-diff")
	if snap || snapDiff {
		fetchAndPrintSnapshot(client, base, token, tabID, snapDiff)
	}
	text, _ := cmd.Flags().GetBool("text")
	if text {
		fetchAndPrintText(client, base, token, tabID)
	}
}

// Forward navigates the current (or specified) tab forward in history.
func Forward(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/forward"
	if tabID != "" {
		path = "/tabs/" + tabID + "/forward"
	}
	path = appendDismissBannersQuery(path, cmd)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPost(client, base, token, path, nil)
		return
	}

	result := apiclient.DoPostQuiet(client, base, token, path, nil)
	if url, ok := result["url"].(string); ok {
		output.Value(url)
	} else {
		output.Success()
	}

	snap, _ := cmd.Flags().GetBool("snap")
	snapDiff, _ := cmd.Flags().GetBool("snap-diff")
	if snap || snapDiff {
		fetchAndPrintSnapshot(client, base, token, tabID, snapDiff)
	}
	text, _ := cmd.Flags().GetBool("text")
	if text {
		fetchAndPrintText(client, base, token, tabID)
	}
}

// Reload reloads the current (or specified) tab.
func Reload(client *http.Client, base, token string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/reload"
	if tabID != "" {
		path = "/tabs/" + tabID + "/reload"
	}
	path = appendDismissBannersQuery(path, cmd)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPost(client, base, token, path, nil)
		return
	}

	apiclient.DoPostQuiet(client, base, token, path, nil)
	output.Success()

	snap, _ := cmd.Flags().GetBool("snap")
	snapDiff, _ := cmd.Flags().GetBool("snap-diff")
	if snap || snapDiff {
		fetchAndPrintSnapshot(client, base, token, tabID, snapDiff)
	}
	text, _ := cmd.Flags().GetBool("text")
	if text {
		fetchAndPrintText(client, base, token, tabID)
	}
}

func Navigate(client *http.Client, base, token string, url string, cmd *cobra.Command) string {
	req := buildNavigateRequest(url, cmd)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		result := postNavigate(client, base, token, req, true)
		apiclient.SuggestNextAction("navigate", result)
		return tabIDFromNavigateResult(result)
	}

	result := postNavigate(client, base, token, req, false)
	resultTabID := tabIDFromNavigateResult(result)
	if resultTabID != "" {
		fmt.Println(resultTabID)
	}

	if !isIdentifiedCaller() {
		output.Hint("no session set — this tab is shared. Create one with: export PINCHTAB_SESSION=$(pinchtab session create --agent-id <id>)")
	}

	snap, _ := cmd.Flags().GetBool("snap")
	snapDiff, _ := cmd.Flags().GetBool("snap-diff")
	if snap || snapDiff {
		fetchAndPrintSnapshot(client, base, token, resultTabID, snapDiff)
	}

	return resultTabID
}

type navigateRequest struct {
	path               string
	body               map[string]any
	fallbackOnNotFound bool
}

func buildNavigateRequest(url string, cmd *cobra.Command) navigateRequest {
	body := map[string]any{"url": url}
	newTab, _ := cmd.Flags().GetBool("new-tab")
	if newTab {
		body["newTab"] = true
	}
	if v, _ := cmd.Flags().GetBool("block-images"); v {
		body["blockImages"] = true
	}
	if v, _ := cmd.Flags().GetBool("block-ads"); v {
		body["blockAds"] = true
	}
	if v, _ := cmd.Flags().GetBool("dismiss-banners"); v {
		body["dismissBanners"] = true
	}
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/navigate"
	explicitTab := cmd.Flags().Changed("tab")
	fallbackOnNotFound := false
	// Don't use tab-specific path when creating a new tab. If the tab came from
	// the saved current-tab state file and no longer exists, retry through
	// /navigate so the server can create/select a current tab. Explicit --tab
	// remains strict and surfaces the 404.
	if tabID != "" && !newTab {
		path = "/tabs/" + tabID + "/navigate"
		fallbackOnNotFound = !explicitTab
	}

	return navigateRequest{
		path:               path,
		body:               body,
		fallbackOnNotFound: fallbackOnNotFound,
	}
}

func postNavigate(client *http.Client, base, token string, req navigateRequest, printResponse bool) map[string]any {
	statusCode, respBody, result := apiclient.DoPostQuietWithStatus(client, base, token, req.path, req.body)
	if statusCode == http.StatusNotFound && req.fallbackOnNotFound {
		statusCode, respBody, result = apiclient.DoPostQuietWithStatus(client, base, token, "/navigate", req.body)
	}
	if statusCode >= 400 {
		apiclient.ExitWithAPIError(statusCode, respBody)
	}
	if printResponse {
		return apiclient.PrintAndDecode(respBody)
	}
	return result
}

func isIdentifiedCaller() bool {
	return strings.TrimSpace(os.Getenv("PINCHTAB_SESSION")) != "" ||
		strings.TrimSpace(os.Getenv("PINCHTAB_AGENT_ID")) != ""
}

func tabIDFromNavigateResult(result map[string]any) string {
	if tid, ok := result["tabId"].(string); ok && tid != "" {
		return tid
	}
	return ""
}

// appendDismissBannersQuery appends ?dismissBanners=true (or &dismissBanners=true)
// to the given path when the cobra command's --dismiss-banners flag is set.
// Used by /back, /forward, /reload which don't carry a JSON body.
func appendDismissBannersQuery(path string, cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetBool("dismiss-banners")
	if !v {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "dismissBanners=true"
}
