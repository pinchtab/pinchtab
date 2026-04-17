package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

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
		if code >= 400 {
			return resultFromBytes(body, code)
		}

		var resp map[string]any
		if err := json.Unmarshal(body, &resp); err != nil {
			return resultFromBytes(body, code)
		}

		bestRef, _ := resp["best_ref"].(string)
		bestRef = strings.TrimSpace(bestRef)
		if bestRef != "" {
			resp["bestRef"] = bestRef
			resp["selector"] = bestRef
			resp["nextActionHint"] = fmt.Sprintf("Use selector %q in pinchtab_click/type/fill/hover. Reuse this ref until page changes; refresh snapshot only after navigation or stale-ref errors.", bestRef)
		} else {
			resp["nextActionHint"] = "No high-confidence ref found. Consider pinchtab_snapshot compact mode once, then retry pinchtab_find with a more specific query."
		}

		return jsonResult(resp)
	}
}
