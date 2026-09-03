package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
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
		body, code, err := c.Post(ctx, routedPathWithBody(r, "/navigate", payload), payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}

		return withOptionalSnapshot(ctx, c, r, tabID, body, code)
	}
}

// withOptionalSnapshot appends the post-navigation snapshot when snap is set, so a
// client can act and see the result in one round-trip. The tab is taken from the
// request when given and otherwise from the tab the response reports, which is what
// makes snap work for a call that did not name a tab. A snapshot failure is not
// fatal: the navigation already happened and its result is the answer.
func withOptionalSnapshot(ctx context.Context, c *Client, r mcp.CallToolRequest, tabID string, body []byte, code int) (*mcp.CallToolResult, error) {
	snap, ok := optBool(r, "snap")
	if !ok || !snap {
		return resultFromBytes(body, code)
	}
	q := url.Values{}
	q.Set("filter", "interactive")
	q.Set("format", "compact")
	if tabID != "" {
		q.Set("tabId", tabID)
	} else if returnedTabID := responseStringField(body, "tabId"); returnedTabID != "" {
		q.Set("tabId", returnedTabID)
	}
	snapBody, _, snapErr := c.GetCapturingVocab(ctx, "/snapshot", routedQuery(r, q), tabID)
	if snapErr != nil {
		return resultFromBytes(body, code)
	}
	return mcp.NewToolResultText(string(body) + "\n" + string(snapBody)), nil
}

// handleHistoryNav serves pinchtab_back, pinchtab_forward and pinchtab_reload. The
// verb is data: the three differ only in the route they post to, so one handler
// takes it as a parameter rather than three near-identical copies drifting apart.
//
// Two things differ from handleNavigate, both forced by the endpoint. /back,
// /forward and /reload never parse a request body — handleHistoryNav reads tabId
// from the query and the tab-scoped route from the path — so no body is sent at
// all, and browser travels in the query via routedPath. Putting browser in a body
// the endpoint does not read would be silently dropped.
func handleHistoryNav(c *Client, verb string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tabID := optString(r, "tabId")
		path := "/" + verb
		if tabID != "" {
			path = "/tabs/" + url.PathEscape(tabID) + "/" + verb
		}
		body, code, err := c.Post(ctx, routedPath(r, path), nil)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}
		return withOptionalSnapshot(ctx, c, r, tabID, body, code)
	}
}

func responseStringField(body []byte, field string) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	value, _ := payload[field].(string)
	return strings.TrimSpace(value)
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
		format, err := snapshotFormatFor(r)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		q.Set("format", format)
		if v, ok := optBool(r, "diff"); ok && v {
			q.Set("diff", "true")
		}
		if sel := optString(r, "selector"); sel != "" {
			q.Set("selector", sel)
		}
		if v, ok := optInt(r, "maxTokens"); ok && v > 0 {
			q.Set("maxTokens", strconv.Itoa(v))
		}
		if v, ok := optInt(r, "depth"); ok && v > 0 {
			q.Set("depth", strconv.Itoa(v))
		}
		if v, ok := optBool(r, "noAnimations"); ok && v {
			q.Set("noAnimations", "true")
		}
		body, code, err := c.GetCapturingVocab(ctx, "/snapshot", routedQuery(r, q), optString(r, "tabId"))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func snapshotFormatFor(r mcp.CallToolRequest) (string, error) {
	if raw := optString(r, "format"); raw != "" {
		return normalizeSnapshotFormat(raw)
	}
	if compact, ok := optBool(r, "compact"); ok && !compact {
		return "json", nil
	}
	return "compact", nil
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
			body, code, err := c.Get(ctx, "/frame", routedQuery(r, q))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return resultFromBytes(body, code)
		}

		payload := map[string]any{"target": target}
		if tabID != "" {
			payload["tabId"] = tabID
		}
		body, code, err := c.Post(ctx, routedPath(r, "/frame"), payload)
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
		if scale, ok := optFloat(r, "scale"); ok && scale > 0 {
			q.Set("scale", fmt.Sprintf("%g", scale))
		}
		if v, ok := optBool(r, "beyondViewport"); ok && v {
			q.Set("beyondViewport", "true")
		}
		annotate := false
		if v, ok := optBool(r, "annotate"); ok && v {
			q.Set("annotate", "true")
			annotate = true
		}
		if quality, ok := optFloat(r, "quality"); ok {
			q.Set("quality", fmt.Sprintf("%d", int(quality)))
		}
		// Always request the inline base64 envelope screenshotResult parses,
		// rather than relying on the server's default output mode.
		q.Set("output", "inline")
		body, code, err := c.Get(ctx, "/screenshot", routedQuery(r, q))
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

// handleCapture implements pinchtab_capture: a paired screenshot + accessibility
// snapshot from the same DOM epoch. The image is delivered as an MCP image
// block; the text block is a JSON envelope containing epoch/pairing metadata
// and the snapshot nodes so a vision-capable model can overlay refs on pixels.
func handleCapture(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		q := url.Values{}
		q.Set("output", "inline")

		if tabID := optString(r, "tabId"); tabID != "" {
			q.Set("tabId", tabID)
		}
		if selector := optString(r, "selector"); selector != "" {
			q.Set("selector", selector)
		}
		if filter := optString(r, "filter"); filter != "" {
			q.Set("filter", filter)
		}
		if format := optString(r, "format"); format != "" {
			q.Set("format", format)
		}
		if wait := optString(r, "wait"); wait != "" {
			q.Set("wait", wait)
		}
		if quality, ok := optFloat(r, "quality"); ok {
			q.Set("quality", fmt.Sprintf("%d", int(quality)))
		}
		if scale, ok := optFloat(r, "scale"); ok && scale > 0 {
			q.Set("scale", fmt.Sprintf("%g", scale))
		}
		if v, ok := optInt(r, "depth"); ok && v > 0 {
			q.Set("depth", strconv.Itoa(v))
		}
		if v, ok := optBool(r, "beyondViewport"); ok && v {
			q.Set("beyondViewport", "true")
		}
		if v, ok := optBool(r, "requirePair"); ok && v {
			q.Set("requirePair", "true")
		}
		if v, ok := optBool(r, "withBounds"); ok && !v {
			q.Set("withBounds", "false")
		}
		if v, ok := optBool(r, "noAnimations"); ok && v {
			q.Set("noAnimations", "true")
		}
		body, code, err := c.Get(ctx, "/capture", routedQuery(r, q))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}
		return captureResult(body)
	}
}

// captureResult parses /capture's inline envelope and produces a paired MCP
// result: image block + JSON text block. On parse failure we fall back to the
// raw bytes so downstream callers still see the wire response.
func captureResult(body []byte) (*mcp.CallToolResult, error) {
	var env struct {
		Status     string          `json:"status"`
		TabID      string          `json:"tabId"`
		URL        string          `json:"url"`
		Title      string          `json:"title"`
		CapturedAt string          `json:"capturedAt"`
		Epoch      json.RawMessage `json:"epoch"`
		Pairing    json.RawMessage `json:"pairing"`
		Image      struct {
			Format          string          `json:"format"`
			Base64          string          `json:"base64"`
			Bytes           int             `json:"bytes"`
			CoordinateSpace string          `json:"coordinateSpace"`
			DPR             float64         `json:"devicePixelRatio"`
			Viewport        json.RawMessage `json:"viewport"`
			Clip            json.RawMessage `json:"clip,omitempty"`
		} `json:"image"`
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Image.Base64 == "" {
		return resultFromBytes(body, 200)
	}

	format := strings.ToLower(strings.TrimSpace(env.Image.Format))
	if format != "png" {
		format = "jpeg"
	}
	mimeType := "image/jpeg"
	if format == "png" {
		mimeType = "image/png"
	}

	imagePayload := map[string]any{
		"format":           format,
		"bytes":            env.Image.Bytes,
		"coordinateSpace":  env.Image.CoordinateSpace,
		"devicePixelRatio": env.Image.DPR,
		"viewport":         env.Image.Viewport,
	}
	if len(env.Image.Clip) > 0 {
		imagePayload["clip"] = env.Image.Clip
	}
	textPayload := map[string]any{
		"status":     env.Status,
		"tabId":      env.TabID,
		"url":        env.URL,
		"title":      env.Title,
		"capturedAt": env.CapturedAt,
		"epoch":      env.Epoch,
		"pairing":    env.Pairing,
		"image":      imagePayload,
		"snapshot":   env.Snapshot,
	}
	encoded, err := json.Marshal(textPayload)
	if err != nil {
		return resultFromBytes(body, 200)
	}
	return mcp.NewToolResultImage(string(encoded), env.Image.Base64, mimeType), nil
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
		if v, ok := optInt(r, "maxChars"); ok && v > 0 {
			q.Set("maxChars", strconv.Itoa(v))
		}
		body, code, err := c.Get(ctx, "/text", routedQuery(r, q))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}
