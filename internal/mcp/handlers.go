package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// maxWaitMS caps wait/timeout durations for safety.
const maxWaitMS = 30_000

func handlerMap(c *Client) map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"pinchtab_navigate":   handleNavigate(c),
		"pinchtab_snapshot":   handleSnapshot(c),
		"pinchtab_frame":      handleFrame(c),
		"pinchtab_screenshot": handleScreenshot(c),
		"pinchtab_capture":    handleCapture(c),
		"pinchtab_get_text":   handleGetText(c),

		"pinchtab_click":            handleAction(c, "click"),
		"pinchtab_type":             handleAction(c, "type"),
		"pinchtab_press":            handleAction(c, "press"),
		"pinchtab_hover":            handleAction(c, "hover"),
		"pinchtab_focus":            handleAction(c, "focus"),
		"pinchtab_select":           handleAction(c, "select"),
		"pinchtab_scroll":           handleAction(c, "scroll"),
		"pinchtab_scroll_into_view": handleAction(c, "scrollintoview"),
		"pinchtab_fill":             handleAction(c, "fill"),

		"pinchtab_keyboard_type":       handleKeyboardText(c, "keyboard-type"),
		"pinchtab_keyboard_inserttext": handleKeyboardText(c, "keyboard-inserttext"),
		"pinchtab_keydown":             handleKeyboardKey(c, "keydown"),
		"pinchtab_keyup":               handleKeyboardKey(c, "keyup"),

		"pinchtab_eval": handleEval(c),
		"pinchtab_pdf":  handlePDF(c),
		"pinchtab_find": handleFind(c),

		"pinchtab_list_tabs":       handleListTabs(c),
		"pinchtab_close_tab":       handleCloseTab(c),
		"pinchtab_health":          handleHealth(c),
		"pinchtab_cookies":         handleCookies(c),
		"pinchtab_connect_profile": handleConnectProfile(c),

		"pinchtab_wait":              handleWait(),
		"pinchtab_wait_for_selector": handleWaitForSelector(c),
		"pinchtab_wait_for_text":     handleWaitForText(c),
		"pinchtab_wait_for_url":      handleWaitForURL(c),
		"pinchtab_wait_for_load":     handleWaitForLoad(c),
		"pinchtab_wait_for_function": handleWaitForFunction(c),

		"pinchtab_network":         handleNetwork(c),
		"pinchtab_network_detail":  handleNetworkDetail(c),
		"pinchtab_network_clear":   handleNetworkClear(c),
		"pinchtab_network_route":   handleNetworkRoute(c),
		"pinchtab_network_unroute": handleNetworkUnroute(c),

		"pinchtab_record_start":  handleRecordStart(c),
		"pinchtab_record_stop":   handleRecordStop(c),
		"pinchtab_record_status": handleRecordStatus(c),

		"pinchtab_dialog": handleDialog(c),
	}
}
