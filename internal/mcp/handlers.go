package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/urlutil"
)

// maxWaitMS caps wait/timeout durations for safety.
const maxWaitMS = 30_000

// handlerMap returns a name→handler map for all PinchTab MCP tools.
func handlerMap(c *Client) map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){
		// Navigation
		"pinchtab_navigate":   handleNavigate(c),
		"pinchtab_snapshot":   handleSnapshot(c),
		"pinchtab_screenshot": handleScreenshot(c),
		"pinchtab_get_text":   handleGetText(c),

		// Interaction
		"pinchtab_click":  handleAction(c, "click"),
		"pinchtab_type":   handleAction(c, "type"),
		"pinchtab_press":  handleAction(c, "press"),
		"pinchtab_hover":  handleAction(c, "hover"),
		"pinchtab_focus":  handleAction(c, "focus"),
		"pinchtab_select": handleAction(c, "select"),
		"pinchtab_scroll": handleAction(c, "scroll"),
		"pinchtab_fill":   handleAction(c, "fill"),

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

// ── helpers ────────────────────────────────────────────────────────────

func optString(r mcp.CallToolRequest, key string) string {
	v, _ := r.GetArguments()[key].(string)
	return v
}

func optFloat(r mcp.CallToolRequest, key string) (float64, bool) {
	v, ok := r.GetArguments()[key].(float64)
	return v, ok
}

func optBool(r mcp.CallToolRequest, key string) (bool, bool) {
	v, ok := r.GetArguments()[key].(bool)
	return v, ok
}

func resultFromBytes(body []byte, code int) (*mcp.CallToolResult, error) {
	if code >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf("HTTP %d: %s", code, string(body))), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

type profileInstanceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Status  string `json:"status"`
	Port    string `json:"port"`
	ID      string `json:"id"`
	Error   string `json:"error"`
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// ── Navigation handlers ────────────────────────────────────────────────

func handleNavigate(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		u, err := r.RequireString("url")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		safeURL, err := urlutil.Sanitize(u)
		if err != nil {
			return mcp.NewToolResultError("invalid URL: " + err.Error()), nil
		}
		payload := map[string]any{"url": safeURL}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/navigate", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleSnapshot(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if v, ok := optBool(r, "interactive"); ok && v {
			q.Set("interactive", "true")
		}
		if v, ok := optBool(r, "compact"); ok && v {
			q.Set("compact", "true")
		}
		if v, ok := optBool(r, "diff"); ok && v {
			q.Set("diff", "true")
		}
		if sel := optString(r, "selector"); sel != "" {
			q.Set("selector", sel)
		}
		body, code, err := c.Get(ctx, "/snapshot", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleScreenshot(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if format := optString(r, "format"); format != "" {
			q.Set("format", format)
		}
		if quality, ok := optFloat(r, "quality"); ok {
			q.Set("quality", fmt.Sprintf("%d", int(quality)))
		}
		body, code, err := c.Get(ctx, "/screenshot", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleGetText(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if v, ok := optBool(r, "raw"); ok && v {
			q.Set("mode", "raw")
		}
		body, code, err := c.Get(ctx, "/text", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// ── Interaction handlers ───────────────────────────────────────────────

func handleAction(c *Client, kind string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]any{"kind": kind}

		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}

		// Unified selector: prefer "selector" param, fall back to legacy "ref".
		resolveSelector := func(required bool) error {
			sel := optString(r, "selector")
			ref := optString(r, "ref")
			if sel != "" {
				payload["selector"] = sel
			} else if ref != "" {
				// Legacy: promote ref to selector
				payload["selector"] = ref
			} else if required {
				return fmt.Errorf("required parameter 'selector' is missing")
			}
			return nil
		}

		switch kind {
		case "click", "hover", "focus":
			if err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

		case "type":
			if err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			text, err := r.RequireString("text")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload["text"] = text

		case "press":
			key, err := r.RequireString("key")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload["key"] = key

		case "select":
			if err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			value, err := r.RequireString("value")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload["value"] = value

		case "scroll":
			_ = resolveSelector(false)
			if px, ok := optFloat(r, "pixels"); ok {
				payload["scrollY"] = int(px)
			}

		case "fill":
			if err := resolveSelector(true); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			value, err := r.RequireString("value")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload["value"] = value
		}

		body, code, err := c.Post(ctx, "/action", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// ── Keyboard handlers (no selector) ────────────────────────────────────

func handleKeyboardText(c *Client, kind string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := r.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"kind": kind, "text": text}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/action", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleKeyboardKey(c *Client, kind string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := r.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"kind": kind, "key": key}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/action", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// ── Content handlers ───────────────────────────────────────────────────

func handleEval(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		expr, err := r.RequireString("expression")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"expression": expr}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/evaluate", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handlePDF(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if v, ok := optBool(r, "landscape"); ok && v {
			q.Set("landscape", "true")
		}
		if scale, ok := optFloat(r, "scale"); ok {
			q.Set("scale", fmt.Sprintf("%.2f", scale))
		}
		if pr := optString(r, "pageRanges"); pr != "" {
			q.Set("pageRanges", pr)
		}
		body, code, err := c.Get(ctx, "/pdf", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleFind(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := r.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"query": query}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/find", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// ── Tab management handlers ────────────────────────────────────────────

func handleListTabs(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		body, code, err := c.Get(ctx, "/tabs", nil)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleCloseTab(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]any{"action": "close"}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/tab", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleHealth(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		body, code, err := c.Get(ctx, "/health", nil)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleCookies(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		body, code, err := c.Get(ctx, "/cookies", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleConnectProfile(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		profile, err := r.RequireString("profile")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		body, code, err := c.Get(ctx, c.profileInstancePath(profile), nil)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}

		var status profileInstanceStatus
		if err := json.Unmarshal(body, &status); err != nil {
			return resultFromBytes(body, code)
		}

		resp := map[string]any{
			"profile": status.Name,
			"running": status.Running,
			"status":  status.Status,
			"id":      status.ID,
			"port":    status.Port,
		}
		if status.Error != "" {
			resp["error"] = status.Error
		}
		if status.Running && status.Port != "" {
			resp["url"] = c.dashboardProfilesURL()
			resp["message"] = fmt.Sprintf("Open the dashboard to access the running profile %q.", status.Name)
			return jsonResult(resp)
		}

		if status.Status == "starting" {
			resp["message"] = fmt.Sprintf("Profile %q is starting; no connect URL is available yet.", status.Name)
		} else {
			resp["message"] = fmt.Sprintf("Profile %q does not have a running instance.", status.Name)
		}
		return jsonResult(resp)
	}
}

// ── Utility handlers ───────────────────────────────────────────────────

func handleWait() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ms, err := r.RequireFloat("ms")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if ms < 0 {
			ms = 0
		}
		if ms > maxWaitMS {
			ms = maxWaitMS
		}
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return mcp.NewToolResultText(fmt.Sprintf(`{"waited_ms":%d}`, int(ms))), nil
		case <-ctx.Done():
			return mcp.NewToolResultError("wait cancelled"), nil
		}
	}
}

func handleWaitForSelector(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sel, err := r.RequireString("selector")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"selector": sel}
		if t, ok := optFloat(r, "timeout"); ok {
			payload["timeout"] = int(t)
		}
		if state := optString(r, "state"); state != "" {
			payload["state"] = state
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/wait", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleWaitForText(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := r.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"text": text}
		if t, ok := optFloat(r, "timeout"); ok {
			payload["timeout"] = int(t)
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/wait", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleWaitForURL(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		u, err := r.RequireString("url")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"url": u}
		if t, ok := optFloat(r, "timeout"); ok {
			payload["timeout"] = int(t)
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/wait", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleWaitForLoad(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		load, err := r.RequireString("load")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"load": load}
		if t, ok := optFloat(r, "timeout"); ok {
			payload["timeout"] = int(t)
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/wait", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleWaitForFunction(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		fn, err := r.RequireString("fn")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"fn": fn}
		if t, ok := optFloat(r, "timeout"); ok {
			payload["timeout"] = int(t)
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/wait", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// ── Network monitoring handlers ────────────────────────────────────────

func handleNetwork(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if filter := optString(r, "filter"); filter != "" {
			q.Set("filter", filter)
		}
		if method := optString(r, "method"); method != "" {
			q.Set("method", method)
		}
		if status := optString(r, "status"); status != "" {
			q.Set("status", status)
		}
		if typ := optString(r, "type"); typ != "" {
			q.Set("type", typ)
		}
		if limit, ok := optFloat(r, "limit"); ok {
			q.Set("limit", fmt.Sprintf("%d", int(limit)))
		}
		if bufSize, ok := optFloat(r, "bufferSize"); ok {
			q.Set("bufferSize", fmt.Sprintf("%d", int(bufSize)))
		}
		body, code, err := c.Get(ctx, "/network", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleNetworkDetail(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		requestID, err := r.RequireString("requestId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if v, ok := optBool(r, "body"); ok && v {
			q.Set("body", "true")
		}
		path := "/network/" + url.PathEscape(requestID)
		body, code, err := c.Get(ctx, path, q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleNetworkClear(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		// POST /network/clear with tabId as query param
		body, code, err := c.Post(ctx, "/network/clear?"+q.Encode(), nil)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// ── Dialog handlers ────────────────────────────────────────────────────

func handleDialog(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		action, err := r.RequireString("action")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if action != "accept" && action != "dismiss" {
			return mcp.NewToolResultError("action must be 'accept' or 'dismiss'"), nil
		}
		payload := map[string]any{"action": action}
		if text := optString(r, "text"); text != "" {
			payload["text"] = text
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/dialog", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}
