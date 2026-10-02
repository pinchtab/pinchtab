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
		if v, ok := optBool(r, "awaitPromise"); ok && v {
			payload["awaitPromise"] = true
		}
		return toolResult(c.Post(ctx, "/evaluate", payload))
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
		return toolResult(c.Get(ctx, "/pdf", q))
	}
}

func handleFind(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := r.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"query": query}
		tabID := optString(r, "tabId")
		if tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.PostCapturingVocab(ctx, "/find", payload, tabID)
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
		tied, _ := resp["tied_refs"].([]any)
		switch {
		case len(tied) > 0:
			resp["nextActionHint"] = fmt.Sprintf("Ambiguous: refs %v match equally well, so none is safe to act on. Retry pinchtab_find with query %q, where i counts from 1 in page order.", tied, resp["retry_query"])
		case bestRef != "":
			resp["bestRef"] = bestRef
			resp["selector"] = bestRef
			resp["nextActionHint"] = fmt.Sprintf("Use selector %q in pinchtab_click/type/fill/hover. Reuse this ref until page changes; refresh snapshot only after navigation or stale-ref errors.", bestRef)
		default:
			resp["nextActionHint"] = "No high-confidence ref found. Consider pinchtab_snapshot compact mode once, then retry pinchtab_find with a more specific query."
		}

		return jsonResult(resp)
	}
}

func handleExtract(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		schema, ok := r.GetArguments()["schema"]
		if !ok || schema == nil {
			return mcp.NewToolResultError("required argument \"schema\" not found"), nil
		}
		payload := map[string]any{"schema": schema}
		tabID := optString(r, "tabId")
		if tabID != "" {
			payload["tabId"] = tabID
		}
		if scope := optTrimmedString(r, "scope"); scope != "" {
			payload["scope"] = scope
		}
		if maxItems, ok := optInt(r, "maxItems"); ok && maxItems > 0 {
			payload["maxItems"] = maxItems
		}
		return toolResult(c.PostCapturingVocab(ctx, "/extract", payload, tabID))
	}
}
