package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// maxWaitMS caps wait/timeout durations for safety.
const maxWaitMS = 30_000

// handlerMap is the only place tool handlers are assembled, so the argument
// checks cannot be bypassed by a caller — production or test — that builds its
// own handler set.
func handlerMap(c *Client) map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw := rawHandlerMap(c)
	checked := make(map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), len(raw))
	for name, h := range raw {
		checked[name] = withTypedArgChecks(name, h)
	}
	return checked
}

func rawHandlerMap(c *Client) map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"pinchtab_navigate":   handleNavigate(c),
		"pinchtab_back":       handleHistoryNav(c, "back"),
		"pinchtab_forward":    handleHistoryNav(c, "forward"),
		"pinchtab_reload":     handleHistoryNav(c, "reload"),
		"pinchtab_snapshot":   handleSnapshot(c),
		"pinchtab_frame":      handleFrame(c),
		"pinchtab_screenshot": handleScreenshot(c),
		"pinchtab_capture":    handleCapture(c),
		"pinchtab_get_text":   handleGetText(c),

		"pinchtab_click":            handleAction(c, "click"),
		"pinchtab_type":             handleAction(c, "type"),
		"pinchtab_hover":            handleAction(c, "hover"),
		"pinchtab_focus":            handleAction(c, "focus"),
		"pinchtab_select":           handleAction(c, "select"),
		"pinchtab_scroll":           handleAction(c, "scroll"),
		"pinchtab_scroll_into_view": handleAction(c, "scrollintoview"),
		"pinchtab_fill":             handleAction(c, "fill"),

		"pinchtab_key": handleKeyboard(c),

		"pinchtab_eval": handleEval(c),
		"pinchtab_pdf":  handlePDF(c),
		"pinchtab_find": handleFind(c),

		"pinchtab_list_tabs":       handleListTabs(c),
		"pinchtab_close_tab":       handleCloseTab(c),
		"pinchtab_health":          handleHealth(c),
		"pinchtab_cookies":         handleCookies(c),
		"pinchtab_cookies_set":     handleCookiesSet(c),
		"pinchtab_connect_profile": handleConnectProfile(c),

		"pinchtab_wait": handleWait(c),

		"pinchtab_network":         handleNetwork(c),
		"pinchtab_network_detail":  handleNetworkDetail(c),
		"pinchtab_network_clear":   handleNetworkClear(c),
		"pinchtab_network_route":   handleNetworkRoute(c),
		"pinchtab_network_unroute": handleNetworkUnroute(c),

		"pinchtab_console": handleConsole(c),
		"pinchtab_errors":  handleErrors(c),

		"pinchtab_record": handleRecord(c),

		"pinchtab_dialog": handleDialog(c),

		"pinchtab_scrape": handleScrape(c),
	}
}
