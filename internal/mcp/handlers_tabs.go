package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

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
		payload := map[string]any{}
		if tabID := optTrimmedString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, "/close", payload)
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

// handleCookiesSet posts one cookie. Every argument the tool declares is read here,
// and every argument read here is declared: an undeclared argument is invisible to a
// model and bypasses the schema-derived validator.
func handleCookiesSet(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := r.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		value, err := r.RequireString("value")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cookie := map[string]any{"name": name, "value": value}
		for arg, key := range map[string]string{
			"domain":   "domain",
			"path":     "path",
			"sameSite": "sameSite",
		} {
			if v := optString(r, arg); v != "" {
				cookie[key] = v
			}
		}
		for _, arg := range []string{"secure", "httpOnly"} {
			if v, ok := optBool(r, arg); ok {
				cookie[arg] = v
			}
		}
		if expires, ok := optFloat(r, "expires"); ok {
			cookie["expires"] = expires
		}

		body := map[string]any{"cookies": []any{cookie}}
		if tabID := optString(r, "tabId"); tabID != "" {
			body["tabId"] = tabID
		}
		if target := optString(r, "url"); target != "" {
			body["url"] = target
		}

		respBody, code, err := c.Post(ctx, "/cookies", body)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// /cookies answers 200 whether or not the browser stored anything, so
		// resultFromBytes alone would report a cookie that was never set as a success —
		// and this is the surface where nothing reads the body afterwards.
		if unset := unsetCookieReport(respBody); unset != "" {
			return mcp.NewToolResultError(fmt.Sprintf("cookie %q was not set: %s", name, unset)), nil
		}
		return resultFromBytes(respBody, code)
	}
}

// unsetCookieReport returns a reason when a /cookies response shows fewer cookies stored
// than were sent, or "" when every one landed. A response it cannot read counts as unset:
// the point is to confirm the write, and an unparseable answer confirms nothing.
func unsetCookieReport(respBody []byte) string {
	var resp struct {
		Set      *int `json:"set"`
		Total    *int `json:"total"`
		Failures []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Sprintf("unreadable response: %s", strings.TrimSpace(string(respBody)))
	}
	if resp.Set == nil || resp.Total == nil {
		return fmt.Sprintf("response did not report how many cookies were set: %s", strings.TrimSpace(string(respBody)))
	}
	if *resp.Set >= *resp.Total {
		return ""
	}
	if len(resp.Failures) > 0 {
		return resp.Failures[0].Error
	}
	return fmt.Sprintf("%d of %d stored, with no reason given", *resp.Set, *resp.Total)
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
			"exists":  status.Exists,
			"running": status.Running,
			"status":  status.Status,
			"id":      status.ID,
			"port":    status.Port,
		}
		if status.Error != "" {
			resp["error"] = status.Error
		}
		if !status.Exists || status.Status == "missing" {
			resp["status"] = "missing"
			resp["message"] = status.Message
			if status.Message == "" {
				resp["message"] = fmt.Sprintf("Profile %q does not exist. Creating and authenticating a reusable profile is a human setup step.", status.Name)
			}
			return jsonResult(resp)
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
