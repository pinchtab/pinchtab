package mcp

import (
	"context"
	"encoding/json"
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
		tabID := optString(r, "tabId")
		if tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/navigate", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}

		// If snap=true, append interactive compact snapshot
		if snap, ok := optBool(r, "snap"); ok && snap {
			q := url.Values{}
			q.Set("filter", "interactive")
			q.Set("format", "compact")
			if tabID != "" {
				q.Set("tabId", tabID)
			}
			snapBody, _, snapErr := c.Get(ctx, "/snapshot", q)
			if snapErr == nil {
				return mcp.NewToolResultText(string(body) + "\n" + string(snapBody)), nil
			}
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
		annotate := false
		if v, ok := optBool(r, "annotate"); ok && v {
			q.Set("annotate", "true")
			annotate = true
		}
		if quality, ok := optFloat(r, "quality"); ok {
			q.Set("quality", fmt.Sprintf("%d", int(quality)))
		}
		body, code, err := c.Get(ctx, "/screenshot", q)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}
		return screenshotResult(body, annotate)
	}
}

// screenshotResult turns the /screenshot JSON envelope into an MCP image
// result so clients can render the picture natively. The text portion is
// always a JSON object `{"format", "annotations": [...]}` so downstream
// callers can parse one stable schema regardless of `annotate`. On any parse
// hiccup we fall back to the raw bytes so error envelopes and future fields
// still surface.
func screenshotResult(body []byte, annotate bool) (*mcp.CallToolResult, error) {
	var env struct {
		Format      string          `json:"format"`
		Base64      string          `json:"base64"`
		Annotations json.RawMessage `json:"annotations,omitempty"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Base64 == "" {
		return resultFromBytes(body, 200)
	}

	format := strings.ToLower(strings.TrimSpace(env.Format))
	if format != "png" {
		format = "jpeg"
	}
	mimeType := "image/jpeg"
	if format == "png" {
		mimeType = "image/png"
	}

	annotations := json.RawMessage("[]")
	if annotate && len(env.Annotations) > 0 {
		annotations = env.Annotations
	}
	payload := map[string]any{
		"format":      format,
		"annotations": annotations,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return resultFromBytes(body, 200)
	}

	return mcp.NewToolResultImage(string(encoded), env.Base64, mimeType), nil
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
