package mcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/urls"
)

func handleNavigate(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		u, err := r.RequireString("url")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		safeURL, err := urls.Sanitize(u)
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
			q.Set("filter", "interactive")
		}
		if v, ok := optBool(r, "compact"); ok && v {
			q.Set("format", "compact")
		}
		if rawFormat := optString(r, "format"); rawFormat != "" {
			format, err := normalizeSnapshotFormat(rawFormat)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			q.Set("format", format)
		}
		if v, ok := optBool(r, "diff"); ok && v {
			q.Set("diff", "true")
		}
		if sel := optString(r, "selector"); sel != "" {
			q.Set("selector", sel)
		}
		if v := optNumber(r, "maxTokens"); v > 0 {
			q.Set("maxTokens", formatInt(v))
		}
		if v := optNumber(r, "depth"); v > 0 {
			q.Set("depth", formatInt(v))
		}
		if v, ok := optBool(r, "noAnimations"); ok && v {
			q.Set("noAnimations", "true")
		}
		body, code, err := c.Get(ctx, "/snapshot", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleFrame(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tabID := optString(r, "tabId")
		target := optString(r, "target")
		if strings.TrimSpace(target) == "" {
			q := url.Values{}
			if tabID != "" {
				q.Set("tabId", tabID)
			}
			body, code, err := c.Get(ctx, "/frame", q)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultFromBytes(body, code)
		}

		payload := map[string]any{"target": target}
		if tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/frame", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func normalizeSnapshotFormat(v string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(v))
	switch format {
	case "compact", "text":
		return format, nil
	default:
		return "", fmt.Errorf("format must be 'compact' or 'text'")
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
		if selector := optString(r, "selector"); selector != "" {
			q.Set("selector", selector)
		}
		if v, ok := optBool(r, "css1x"); ok && v {
			q.Set("css1x", "true")
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
		if format := optString(r, "format"); format != "" {
			q.Set("format", format)
		}
		if v := optNumber(r, "maxChars"); v > 0 {
			q.Set("maxChars", formatInt(v))
		}
		body, code, err := c.Get(ctx, "/text", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}
