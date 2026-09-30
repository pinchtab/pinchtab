package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func handleDialog(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rawAction, err := r.RequireString("action")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		action, err := bridge.ParseDialogAction("action", rawAction)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"action": action}
		if text := optString(r, "text"); text != "" {
			payload["text"] = text
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		return toolResult(c.Post(ctx, "/dialog", payload))
	}
}
