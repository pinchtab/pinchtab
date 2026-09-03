package mcp

import (
	"context"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleConsole and handleErrors mirror the CLI's `pinchtab console [--clear]` and
// `pinchtab errors [--clear]`: a GET reads the channel, and the optional clear
// boolean routes to the POST clear endpoint instead. resultFromBytes is the shared
// funnel — /console and /errors are channels whose payload IS failures, and neither
// carries a top-level counting shape, so an error-laden body reaches the agent as a
// successful result rather than being misread as a failed call.
func handleConsole(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return diagnosticsHandler(c, "/console")
}

func handleErrors(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return diagnosticsHandler(c, "/errors")
}

func diagnosticsHandler(c *Client, path string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if clear, ok := optBool(r, "clear"); ok && clear {
			body, code, err := c.Post(ctx, path+"/clear?"+q.Encode(), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultFromBytes(body, code)
		}
		body, code, err := c.Get(ctx, path, q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}
