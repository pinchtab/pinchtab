package actions

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Back navigates the current (or specified) tab back in history.
func Back(client *http.Client, base, token string, cmd *cobra.Command) {
	historyNav(client, base, token, "back", cmd)
}

// Forward navigates the current (or specified) tab forward in history.
func Forward(client *http.Client, base, token string, cmd *cobra.Command) {
	historyNav(client, base, token, "forward", cmd)
}

// Reload reloads the current (or specified) tab.
func Reload(client *http.Client, base, token string, cmd *cobra.Command) {
	historyNav(client, base, token, "reload", cmd)
}

// historyNav is the one body behind back, forward and reload. The server answers
// all three from a single handler returning {"tabId","url"}, so the CLI reports
// the landed URL the same way for each rather than keeping three copies that
// drift — reload used to discard the response and print a bare OK.
func historyNav(client *http.Client, base, token, action string, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	path := "/" + action
	if tabID != "" {
		path = "/tabs/" + tabID + "/" + action
	}
	path = appendDismissBannersQuery(path, cmd)

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoPost(client, base, token, path, nil)
		return
	}

	result := apiclient.DoPostQuiet(client, base, token, path, nil)
	if landed := landedURL(result); landed != "" {
		output.Value(landed)
	} else {
		output.Success()
	}

	printPostActionOutput(client, base, token, tabID, cmd)
}

// printPostActionOutput runs the shared --snap / --snap-diff / --text tail.
func printPostActionOutput(client *http.Client, base, token, tabID string, cmd *cobra.Command) {
	snap, _ := cmd.Flags().GetBool("snap")
	snapDiff, _ := cmd.Flags().GetBool("snap-diff")
	if snap || snapDiff {
		fetchAndPrintSnapshot(client, base, token, tabID, snapDiff)
	}
	if text, _ := cmd.Flags().GetBool("text"); text {
		fetchAndPrintText(client, base, token, tabID)
	}
}

// landedURL is the URL the navigation actually ended on, which is not the
// requested one whenever a redirect, login wall or error page intervened. Empty
// when the response carries none, so callers can stay terse instead of printing
// a blank line.
func landedURL(result map[string]any) string {
	landed, _ := result["url"].(string)
	return landed
}

func Navigate(client *http.Client, base, token string, url string, cmd *cobra.Command) string {
	req := buildNavigateRequest(url, cmd)

	if timeoutSeconds, ok := req.body["timeout"].(float64); ok {
		client = navigationHTTPClient(client, timeoutSeconds)
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		result, usedFallback := postNavigate(client, base, token, req, true)
		resultTabID := tabIDFromNavigateResult(result)
		reportFallbackNewTab(cmd, usedFallback, req.tabID, resultTabID)
		apiclient.SuggestNextAction("navigate", result)
		return resultTabID
	}

	result, usedFallback := postNavigate(client, base, token, req, false)
	resultTabID := tabIDFromNavigateResult(result)
	reportFallbackNewTab(cmd, usedFallback, req.tabID, resultTabID)
	stdout := navigateStdoutFor(cmd)
	if resultTabID != "" {
		if stdout.tabID {
			fmt.Println(resultTabID)
		} else {
			fmt.Fprintln(os.Stderr, resultTabID)
		}
	}
	if stdout.landed {
		if landed := landedURL(result); landed != "" {
			output.Value(landed)
		}
	}

	if !isIdentifiedCaller(cmd) {
		output.Advisory(cli.NoSessionHint)
	}

	printPostActionOutput(client, base, token, resultTabID, cmd)

	return resultTabID
}

type navigateStdout struct {
	tabID  bool
	landed bool
}

func navigateStdoutFor(cmd *cobra.Command) navigateStdout {
	captured := tabIDOnly(cmd)
	return navigateStdout{tabID: !captured || !payloadRequested(cmd), landed: !captured}
}

func payloadRequested(cmd *cobra.Command) bool {
	snap, _ := cmd.Flags().GetBool("snap")
	snapDiff, _ := cmd.Flags().GetBool("snap-diff")
	text, _ := cmd.Flags().GetBool("text")
	return snap || snapDiff || text
}

func tabIDOnly(cmd *cobra.Command) bool {
	if only, _ := cmd.Flags().GetBool("print-tab-id"); only {
		return true
	}
	return !stdoutIsTerminal()
}

// stdoutIsTerminal stands in for an environment a test cannot set, so it is a
// var; production reads its result on every non-JSON navigate.
var stdoutIsTerminal = func() bool {
	info, err := os.Stdout.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

type navigateRequest struct {
	path               string
	body               map[string]any
	fallbackOnNotFound bool
	// tabID is the tab the request was scoped to, kept so a fallback can name
	// the tab that turned out to be gone. Read once, here, rather than again at
	// the reporting site.
	tabID string
}

func buildNavigateRequest(url string, cmd *cobra.Command) navigateRequest {
	body := map[string]any{"url": url}
	newTab, _ := cmd.Flags().GetBool("new-tab")
	if newTab {
		body["newTab"] = true
	}
	if timeoutSeconds := navigationTimeoutSeconds(cmd); timeoutSeconds > 0 {
		body["timeout"] = timeoutSeconds
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
		tabID:              tabID,
	}
}

func navigationTimeoutSeconds(cmd *cobra.Command) float64 {
	timeoutSeconds, err := cmd.Flags().GetFloat64("timeout")
	if err != nil || timeoutSeconds <= 0 || math.IsNaN(timeoutSeconds) {
		return 0
	}
	if timeoutSeconds > httpx.MaxNavigationTimeout.Seconds() {
		return httpx.MaxNavigationTimeout.Seconds()
	}
	return timeoutSeconds
}

func navigationHTTPClient(client *http.Client, timeoutSeconds float64) *http.Client {
	clone := *client
	clone.Timeout = time.Duration(timeoutSeconds*float64(time.Second)) + httpx.NavigationTransportGrace
	return &clone
}

// postNavigate returns the decoded response and whether the tab-scoped request
// 404'd and was retried unscoped, which is the case the caller has to report.
func postNavigate(client *http.Client, base, token string, req navigateRequest, printResponse bool) (map[string]any, bool) {
	statusCode, respBody, result := apiclient.DoPostQuietWithStatus(client, base, token, req.path, req.body)
	usedFallback := false
	if statusCode == http.StatusNotFound && req.fallbackOnNotFound {
		usedFallback = true
		statusCode, respBody, result = apiclient.DoPostQuietWithStatus(client, base, token, "/navigate", req.body)
	}
	if statusCode >= 400 {
		apiclient.ExitWithAPIError(statusCode, respBody)
	}
	if printResponse {
		return apiclient.PrintAndDecode(respBody), usedFallback
	}
	return result, usedFallback
}

// reportFallbackNewTab says so when the fallback left the caller on a second tab.
//
// The retry drops the tab id, which makes it an unscoped request, and the server's
// published contract for an UNSCOPED navigate is caller-identity dependent
// (docs/endpoints.md): an anonymous caller always gets a new tab — CurrentTabStore
// has no entry for a global scope to adopt — while a session or agent-id caller
// reuses that scope's current tab. So the notice is exactly as narrow as the
// guarantee behind it: anonymous callers, where "a new tab was opened" is a fact
// rather than an inference. An identified caller stays silent because nothing was
// created for it to hear about.
//
// The remedy is deliberately absent: an anonymous navigate already prints
// cli.NoSessionHint, which carries the run-with-a-session advice, and repeating it
// here would be the same guidance twice on one command.
func reportFallbackNewTab(cmd *cobra.Command, usedFallback bool, staleTabID, newTabID string) {
	if !usedFallback || isIdentifiedCaller(cmd) {
		return
	}
	if newTabID == "" {
		output.Hint(fmt.Sprintf("tab %s no longer exists — the server opened a new tab for this navigation", staleTabID))
		return
	}
	output.Hint(fmt.Sprintf("tab %s no longer exists — opened a new tab %s for this navigation", staleTabID, newTabID))
}

// isIdentifiedCaller reports whether the server will see this call as scoped to a session
// or an agent, which is what decides whether an unscoped retry adopts a tab or creates one.
//
// The agent id has TWO provenances and this must agree with the CLI's own resolution, which
// prefers the --agent-id persistent flag over the environment. Reading only the environment
// calls a flag-identified caller anonymous, and the retry then names an ADOPTED tab as newly
// opened — the inverse of the defect the notice exists to report.
func isIdentifiedCaller(cmd *cobra.Command) bool {
	return strings.TrimSpace(os.Getenv("PINCHTAB_SESSION")) != "" ||
		strings.TrimSpace(os.Getenv("PINCHTAB_AGENT_ID")) != "" ||
		agentIDFlag(cmd) != ""
}

// agentIDFlag reads the root's persistent --agent-id off whichever command is running.
//
// Both lookups are needed. Flags() carries the flag only once cobra has parsed and merged
// the persistent set, so a command tree built directly — every test here, and any caller
// that has not executed — sees nil there and finds it through InheritedFlags(). Checking
// only Flags() would leave the guard reading empty in exactly the tests meant to pin it.
//
// A command with no parent has no root flags to inherit, so an absent flag means anonymous
// rather than an error: newNavigateCmd() is built standalone.
func agentIDFlag(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		if flag := set.Lookup("agent-id"); flag != nil {
			if v := strings.TrimSpace(flag.Value.String()); v != "" {
				return v
			}
		}
	}
	return ""
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
