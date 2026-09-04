package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/selector"
)

func optString(r mcp.CallToolRequest, key string) string {
	v, _ := r.GetArguments()[key].(string)
	return v
}

func optTrimmedString(r mcp.CallToolRequest, key string) string {
	return strings.TrimSpace(optString(r, key))
}

// Tool calls are written by language models, and a stringified number is one of
// the commonest shapes they emit. Every numeric argument accepts both, so a
// future one cannot pick a strict accessor by accident — there is none.
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

func optInt(r mcp.CallToolRequest, key string) (int, bool) {
	v, ok := optFloat(r, key)
	return int(v), ok
}

// Booleans get the same tolerance as numbers, and for the same reason. The one
// opt-out argument makes it matter more than the opt-in flags: dropping
// withBounds="false" leaves bounds switched on, so the response looks like the
// default rather than like the request.
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

// firstSuppliedString answers "was this argument sent", the question firstNonEmptyString
// cannot answer because collapsing empty and absent is its entire job. It is the same
// question the bridge asks — ActionRequest.HasText is inferred from key presence over the
// same text/value pair that fill and select both read.
//
// The value travels verbatim rather than trimmed, so MCP and POST /action agree on every
// input and not merely on the cases where emptiness is the point.
//
// The dialog arguments keep the collapsing helper deliberately: an empty dialogAction means
// not-specified, so a value this helper would report as supplied-but-unusable is instead a
// skipped one-shot handler. That is a silent SKIP rather than a misleading refusal — a
// different shape, and it wants its own decision rather than being swept in here.
//
// wrongType names the JSON type of a value that WAS supplied under one of the keys but is
// not a string. The library does not enforce the declared type before calling a handler, so
// a model answering a numeric-looking field with 2024 rather than "2024" arrives here — and
// collapsing that into "not supplied" reproduces the very complaint this helper was written
// for: telling a caller an argument it sent is missing.
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

// firstSelectorString is firstNonEmptyString for the selector aliases, except that a key
// which was SENT under a non-string type is reported by name instead of being collapsed
// into not-given. A later alias that yields a usable string still wins — the caller's
// intent is unambiguous there — so wrongKey only survives when NO key produced a selector.
// A supplied empty string keeps collapsing deliberately: empty carries no selector meaning
// on any verb, so it falls through exactly as before.
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

// actionSelectorArg resolves common selector aliases used by MCP clients.
// If only "query" is provided, natural language input is normalized to
// semantic selector form (find:...).
//
// A wrong-typed alias REFUSES rather than falling through — the fall-through would act on
// a different argument (query) or a different target (nodeId) while the caller's mistyped
// selector is silently ignored, which is worse than refusing. The caller of this function
// turns wrongKey/wrongType into the refusal, naming the key that was actually sent.
func actionSelectorArg(r mcp.CallToolRequest) (sel, wrongKey, wrongType string) {
	sel, wrongKey, wrongType = firstSelectorString(r, "selector", "ref", "element", "target")
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

func resultFromBytes(body []byte, code int) (*mcp.CallToolResult, error) {
	if code >= 400 {
		return mcp.NewToolResultError(fmt.Sprintf("HTTP %d: %s", code, string(body))), nil
	}
	if reason := reportsNoSuccess(body); reason != "" {
		return mcp.NewToolResultError(reason), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// reportsNoSuccess is the funnel's body-level failure rule: an endpoint that answers 200
// while reporting it achieved nothing must reach the agent as an error, or the agent
// confirms work the browser never did — the cookie-set path shipped exactly that.
//
// The rule keys on the COUNTING SHAPE, not on failure-flavoured key names: a top-level
// numeric "failed" above zero marks a response that counts the work it was asked to do.
// That is what keeps the exclusions structural rather than a list — the observability
// snapshot, /health/tabs and the console endpoint report failures AS their payload but
// carry no top-level failed count, and /network's per-request "failed" is a nested bool,
// so none of them can match. A key-name rule would break exactly those.
//
// Partial success stays a SUCCESS carrying detail: the succeeded items' effects already
// happened, and the body's own counts and failures list are what the agent needs to retry
// only what missed. Zero success is an error, with the body riding along as the reason.
// Fail closed within the shape: a failed count with no readable success count beside it
// confirms nothing and is refused. A body that does not parse as an object cannot carry
// the shape and keeps the status rule — most funnel responses are not counting anything.
//
// The cookie tool keeps unsetCookieReport ON TOP of this rule deliberately: it confirms a
// single named write with a stricter fail-closed contract (an unreadable body, or one
// missing its counts, refuses) than a funnel serving every tool can impose without
// breaking non-counting responses. Its check runs first, so its more specific message
// wins; this rule is the class-wide net behind it.
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
