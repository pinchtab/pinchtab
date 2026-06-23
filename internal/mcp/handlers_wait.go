package mcp

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

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
		return callWaitEndpoint(ctx, c, r, map[string]any{"selector": sel})
	}
}

func handleWaitForText(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := r.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return callWaitEndpoint(ctx, c, r, map[string]any{"text": text})
	}
}

func handleWaitForURL(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		u, err := r.RequireString("url")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return callWaitEndpoint(ctx, c, r, map[string]any{"url": u})
	}
}

func handleWaitForLoad(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		load, err := r.RequireString("load")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return callWaitEndpoint(ctx, c, r, map[string]any{"load": load})
	}
}

func handleWaitForFunction(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		fn, err := r.RequireString("fn")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return callWaitEndpoint(ctx, c, r, map[string]any{"fn": fn})
	}
}

func callWaitEndpoint(ctx context.Context, c *Client, r mcp.CallToolRequest, payload map[string]any) (*mcp.CallToolResult, error) {
	if timeout, ok := optFloat(r, "timeout"); ok {
		payload["timeout"] = int(timeout)
	}
	if state := optString(r, "state"); state != "" {
		payload["state"] = state
	}
	if tabID := optString(r, "tabId"); tabID != "" {
		payload["tabId"] = tabID
	}
	// Browser routing is query-based (orchestrator.ExtractRequestedBrowser reads
	// r.URL.Query()), so forward `browser` in the query, not the body — a body
	// value would be silently dropped by the router.
	path := "/wait"
	if browser := optString(r, "browser"); browser != "" {
		path += "?browser=" + url.QueryEscape(browser)
	}
	body, code, err := c.Post(ctx, path, payload)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return resultFromBytes(body, code)
}
