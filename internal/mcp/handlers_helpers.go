package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/selector"
)

func optString(r mcp.CallToolRequest, key string) string {
	v, _ := r.GetArguments()[key].(string)
	return v
}

func optTrimmedString(r mcp.CallToolRequest, key string) string {
	return strings.TrimSpace(optString(r, key))
}

// Models commonly send numbers as strings; there is no strict numeric accessor.
func optFloat(r mcp.CallToolRequest, key string) (float64, bool) {
	if v, ok := r.GetArguments()[key].(float64); ok {
		return v, true
	}
	if raw := optTrimmedString(r, key); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

func firstFloat(r mcp.CallToolRequest, keys ...string) (float64, bool) {
	for _, key := range keys {
		if v, ok := optFloat(r, key); ok {
			return v, true
		}
	}
	return 0, false
}

func optInt(r mcp.CallToolRequest, key string) (int, bool) {
	v, ok := optFloat(r, key)
	return int(v), ok
}

// A stringified boolean is accepted so withBounds="false" is not silently read as the default.
func optBool(r mcp.CallToolRequest, key string) (bool, bool) {
	if v, ok := r.GetArguments()[key].(bool); ok {
		return v, true
	}
	if raw := optTrimmedString(r, key); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			return v, true
		}
	}
	return false, false
}

func pickMode[T any](r mcp.CallToolRequest, key string, table map[string]T) (string, T, *mcp.CallToolResult) {
	name := optTrimmedString(r, key)
	if entry, ok := table[name]; ok {
		return name, entry, nil
	}
	names := make([]string, 0, len(table))
	for candidate := range table {
		names = append(names, candidate)
	}
	sort.Strings(names)
	var zero T
	return "", zero, mcp.NewToolResultError(fmt.Sprintf("'%s' must be one of %s, got %q", key, strings.Join(names, ", "), name))
}

func firstNonEmptyString(r mcp.CallToolRequest, keys ...string) string {
	for _, key := range keys {
		if v := optTrimmedString(r, key); v != "" {
			return v
		}
	}
	return ""
}

// firstSuppliedString reports presence, untrimmed, so MCP and POST /action agree on every
// input; wrongType names a value that was sent under a non-string type instead of
// collapsing it into "not supplied".
func firstSuppliedString(r mcp.CallToolRequest, keys ...string) (value string, supplied bool, wrongType string) {
	args := r.GetArguments()
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		if v, isString := raw.(string); isString {
			return v, true, ""
		}
		if wrongType == "" {
			wrongType = jsonTypeName(raw)
		}
	}
	return "", false, wrongType
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64, int, int64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func looksLikeStructuredSelector(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if strings.HasPrefix(v, "#") || strings.HasPrefix(v, ".") || strings.HasPrefix(v, "[") {
		return true
	}
	if strings.HasPrefix(v, "//") || strings.HasPrefix(v, "(//") {
		return true
	}
	if strings.ContainsAny(v, "[]#>+~") || containsSpacelessAny(v, ":=") {
		return true
	}
	// Treat dot notation as CSS only when it looks like tag/class syntax,
	// not plain text like numeric values (e.g. "50.50").
	if strings.Contains(v, ".") && hasASCIIAlpha(v) && !strings.ContainsAny(v, " \t\r\n") {
		return true
	}
	return false
}

func containsSpacelessAny(v, chars string) bool {
	for i := 0; i < len(v); i++ {
		if !strings.ContainsRune(chars, rune(v[i])) {
			continue
		}
		if i > 0 && isASCIISpace(v[i-1]) {
			continue
		}
		if i+1 < len(v) && isASCIISpace(v[i+1]) {
			continue
		}
		return true
	}
	return false
}

func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func hasASCIIAlpha(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

// A later alias with a usable string wins; wrongKey survives only when no key produced a selector.
func firstSelectorString(r mcp.CallToolRequest, keys ...string) (value, wrongKey, wrongType string) {
	args := r.GetArguments()
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		if v, isString := raw.(string); isString {
			if v = strings.TrimSpace(v); v != "" {
				return v, "", ""
			}
			continue
		}
		if wrongKey == "" {
			wrongKey, wrongType = key, jsonTypeName(raw)
		}
	}
	return "", wrongKey, wrongType
}

// A wrong-typed selector alias refuses rather than falling through to a different argument.
func actionSelectorArg(r mcp.CallToolRequest) (sel, wrongKey, wrongType string) {
	sel, wrongKey, wrongType = firstSelectorString(r, selectorArgKeys...)
	if sel != "" || wrongType != "" {
		return sel, wrongKey, wrongType
	}
	if raw, ok := r.GetArguments()["query"]; ok {
		if _, isString := raw.(string); !isString {
			return "", "query", jsonTypeName(raw)
		}
	}
	query := optTrimmedString(r, "query")
	if query == "" {
		return "", "", ""
	}
	if selector.HasKnownPrefix(query) || selector.IsRef(query) || looksLikeStructuredSelector(query) {
		return query, "", ""
	}
	return "find:" + query, "", ""
}

func resolveXY(r mcp.CallToolRequest) (float64, float64, bool) {
	x, okX := optFloat(r, "x")
	y, okY := optFloat(r, "y")
	if okX && okY {
		return x, y, true
	}
	return 0, 0, false
}

func toolResult(body []byte, code int, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return resultFromBytes(body, code)
}

func resultFromBytes(body []byte, code int) (*mcp.CallToolResult, error) {
	if code >= 400 {
		message := fmt.Sprintf("HTTP %d: %s", code, string(body))
		if guidance := mcpRemedyGuidance(body); guidance != "" {
			message = guidance + "\n" + message
		}
		return mcp.NewToolResultError(message), nil
	}
	if reason := reportsNoSuccess(body); reason != "" {
		return mcp.NewToolResultError(reason), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

type remedyTool struct {
	name  string
	param string
}

// Neither tool name nor parameter follows from the CLI verb (nav is pinchtab_navigate,
// dialog's positional is its action), so the table is explicit and the whole-binary remedy
// census pins it complete.
var remedyVerbTools = map[string]remedyTool{
	"resume": {"pinchtab_resume", "tabId"},
	"dialog": {"pinchtab_dialog", "action"},
	"nav":    {"pinchtab_navigate", "url"},
	"back":   {"pinchtab_back", ""},
	"snap":   {"pinchtab_snapshot", ""},
}

// A verb without an MCP counterpart, or a multi-command remedy, yields no guidance.
func mcpRemedyGuidance(body []byte) string {
	segments := remedy.Segments(remedyLine(body))
	if len(segments) != 1 {
		return ""
	}
	words := segments[0]
	if len(words) < 2 {
		return ""
	}
	tool, ok := remedyVerbTools[words[1]]
	if !ok {
		return ""
	}
	if tool.param != "" {
		if arg := firstPositional(words[2:]); arg != "" {
			return fmt.Sprintf("call %s with %s %s", tool.name, tool.param, arg)
		}
	}
	return "call " + tool.name
}

// RemedyToolForVerb is the seam the whole-binary remedy census reads.
func RemedyToolForVerb(verb string) (name, param string, ok bool) {
	tool, ok := remedyVerbTools[verb]
	return tool.name, tool.param, ok
}

// ToolInputHasParam reports whether name is a registered tool declaring param (any tool when param is empty).
func ToolInputHasParam(name, param string) bool {
	for _, tool := range allTools() {
		if tool.Name != name {
			continue
		}
		if param == "" {
			return true
		}
		_, ok := tool.InputSchema.Properties[param]
		return ok
	}
	return false
}

func remedyLine(body []byte) string {
	var envelope struct {
		Details struct {
			Remedy string `json:"remedy"`
		} `json:"details"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	return envelope.Details.Remedy
}

func firstPositional(words []string) string {
	for _, word := range words {
		if !strings.HasPrefix(word, "-") {
			return word
		}
	}
	return ""
}

// A 200 whose top-level counts report zero successes reaches the agent as an error. The
// rule keys on the counting shape, not on key names, so payloads that describe failures
// without a top-level failed count cannot match. Partial success stays a success; a failed
// count with no success count beside it refuses.
func reportsNoSuccess(body []byte) string {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return ""
	}
	failed, ok := topLevelCount(top, "failed")
	if !ok || failed == 0 {
		return ""
	}
	for _, key := range []string{"set", "successful", "succeeded"} {
		succeeded, ok := topLevelCount(top, key)
		if !ok {
			continue
		}
		if succeeded > 0 {
			return ""
		}
		return fmt.Sprintf("the call reported no successes (%s 0, failed %d): %s", key, failed, strings.TrimSpace(string(body)))
	}
	return fmt.Sprintf("the call reported %d failed and no success count to confirm anything landed: %s", failed, strings.TrimSpace(string(body)))
}

func topLevelCount(top map[string]json.RawMessage, key string) (int, bool) {
	raw, ok := top[key]
	if !ok {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return int(n), true
}

type profileInstanceStatus struct {
	Name    string `json:"name"`
	Exists  bool   `json:"exists"`
	Running bool   `json:"running"`
	Status  string `json:"status"`
	Port    string `json:"port"`
	ID      string `json:"id"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}
