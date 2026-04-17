package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// maxWaitMS caps wait/timeout durations for safety.
const maxWaitMS = 30_000

// handlerMap returns a name→handler map for all PinchTab MCP tools.
func handlerMap(c *Client) map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		// Navigation
		"pinchtab_navigate":   handleNavigate(c),
		"pinchtab_snapshot":   handleSnapshot(c),
		"pinchtab_frame":      handleFrame(c),
		"pinchtab_screenshot": handleScreenshot(c),
		"pinchtab_get_text":   handleGetText(c),

		// Interaction
		"pinchtab_click":            handleAction(c, "click"),
		"pinchtab_type":             handleAction(c, "type"),
		"pinchtab_press":            handleAction(c, "press"),
		"pinchtab_hover":            handleAction(c, "hover"),
		"pinchtab_focus":            handleAction(c, "focus"),
		"pinchtab_select":           handleAction(c, "select"),
		"pinchtab_scroll":           handleAction(c, "scroll"),
		"pinchtab_scroll_into_view": handleAction(c, "scrollintoview"),
		"pinchtab_fill":             handleAction(c, "fill"),

		// Keyboard (no selector)
		"pinchtab_keyboard_type":       handleKeyboardText(c, "keyboard-type"),
		"pinchtab_keyboard_inserttext": handleKeyboardText(c, "keyboard-inserttext"),
		"pinchtab_keydown":             handleKeyboardKey(c, "keydown"),
		"pinchtab_keyup":               handleKeyboardKey(c, "keyup"),

		// Content
		"pinchtab_eval": handleEval(c),
		"pinchtab_pdf":  handlePDF(c),
		"pinchtab_find": handleFind(c),

		// Tab management
		"pinchtab_list_tabs":       handleListTabs(c),
		"pinchtab_close_tab":       handleCloseTab(c),
		"pinchtab_health":          handleHealth(c),
		"pinchtab_cookies":         handleCookies(c),
		"pinchtab_connect_profile": handleConnectProfile(c),

		// Utility
		"pinchtab_wait":              handleWait(),
		"pinchtab_wait_for_selector": handleWaitForSelector(c),
		"pinchtab_wait_for_text":     handleWaitForText(c),
		"pinchtab_wait_for_url":      handleWaitForURL(c),
		"pinchtab_wait_for_load":     handleWaitForLoad(c),
		"pinchtab_wait_for_function": handleWaitForFunction(c),

		// Network monitoring
		"pinchtab_network":        handleNetwork(c),
		"pinchtab_network_detail": handleNetworkDetail(c),
		"pinchtab_network_clear":  handleNetworkClear(c),

		// Dialog
		"pinchtab_dialog": handleDialog(c),
	}
}
