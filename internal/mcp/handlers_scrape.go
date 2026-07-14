package mcp

import (
	"context"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// scrapeMCPTimeout bounds a scrape driven over MCP: the HTTP crawl plus any
// browser-rendered pages. Matches the CLI/HTTP client-side scrape budget, well
// above the MCP client's default per-request timeout.
const scrapeMCPTimeout = 15 * time.Minute

func handleScrape(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		u, err := r.RequireString("url")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]any{"url": u}
		if v, ok := optInt(r, "maxPages"); ok {
			payload["maxPages"] = v
		}
		if v, ok := optInt(r, "maxPerPattern"); ok {
			payload["maxPerPattern"] = v
		}
		if v, ok := optInt(r, "concurrency"); ok {
			payload["concurrency"] = v
		}
		if v, ok := optInt(r, "timeoutSeconds"); ok {
			payload["timeoutSeconds"] = v
		}
		if v, ok := optBool(r, "preview"); ok && v {
			payload["preview"] = true
		}
		if v, ok := optBool(r, "enrichAll"); ok && v {
			payload["enrichAll"] = true
		}
		if v, ok := optBool(r, "noBrowser"); ok && v {
			payload["noBrowser"] = true
		}
		if list := splitCommaList(optString(r, "only")); len(list) > 0 {
			payload["only"] = list
		}
		if list := splitCommaList(optString(r, "include")); len(list) > 0 {
			payload["includePatterns"] = list
		}
		if list := splitCommaList(optString(r, "exclude")); len(list) > 0 {
			payload["excludePatterns"] = list
		}
		if browser := optString(r, "browser"); browser != "" {
			payload["browser"] = browser
		}
		// A multi-page scrape legitimately runs for minutes; the default MCP
		// client timeout is far too short for it.
		body, code, err := c.withTimeout(scrapeMCPTimeout).Post(ctx, "/scrape", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

// splitCommaList splits a comma- or newline-separated string into trimmed,
// non-empty items — the MCP-friendly way to pass the scrape URL/pattern lists
// without an array schema.
func splitCommaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
