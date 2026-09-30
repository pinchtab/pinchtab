package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

var schemaPropertiesOnce = sync.OnceValue(func() map[string]map[string]string {
	properties := make(map[string]map[string]string)
	for _, tool := range allTools() {
		properties[tool.Name] = propertiesOf(tool)
	}
	return properties
})

var schemaArgTypesOnce = sync.OnceValue(func() map[string]map[string]string {
	types := make(map[string]map[string]string)
	for name, properties := range schemaPropertiesOnce() {
		types[name] = typedOnly(properties)
	}
	return types
})

func propertiesOf(tool mcp.Tool) map[string]string {
	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return nil
	}
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil
	}

	properties := make(map[string]string, len(schema.Properties))
	for name, property := range schema.Properties {
		properties[name] = property.Type
	}
	return properties
}

func typedOnly(properties map[string]string) map[string]string {
	types := make(map[string]string, len(properties))
	for name, kind := range properties {
		switch kind {
		case "number", "integer", "boolean":
			types[name] = kind
		}
	}
	return types
}

func validateDeclaredArgs(toolName string, args map[string]any) error {
	declared, known := schemaPropertiesOnce()[toolName]
	if !known || len(args) == 0 {
		return nil
	}

	unknown := make([]string, 0, len(args))
	for name, value := range args {
		if _, ok := declared[name]; ok {
			continue
		}
		unknown = append(unknown, "unknown argument "+strconv.Quote(name)+nearestArgHint(declared, name, value))
	}
	if len(unknown) == 0 {
		return nil
	}

	sort.Strings(unknown)
	return fmt.Errorf("%s: %s; declared arguments: %s", toolName, strings.Join(unknown, "; "), declaredArgList(declared))
}

func declaredArgList(declared map[string]string) string {
	if len(declared) == 0 {
		return "none"
	}
	names := make([]string, 0, len(declared))
	for name := range declared {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func nearestArgHint(declared map[string]string, name string, value any) string {
	if raw, ok := value.(string); ok && declared[raw] == "boolean" {
		return fmt.Sprintf(" (did you mean %q: true?)", raw)
	}
	if match := closestArgName(declared, name); match != "" {
		return fmt.Sprintf(" (did you mean %q?)", match)
	}
	return ""
}

func closestArgName(declared map[string]string, name string) string {
	best, bestDistance := "", -1
	for candidate := range declared {
		distance, close := argNameDistance(name, candidate)
		if !close {
			continue
		}
		if bestDistance < 0 || distance < bestDistance || (distance == bestDistance && candidate < best) {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

const minRelatedArgNameLength = 3

func argNameDistance(name, candidate string) (int, bool) {
	a, b := strings.ToLower(name), strings.ToLower(candidate)
	distance := editDistance(a, b)
	stem := sharedPrefixLength(a, b)
	related := stem >= minRelatedArgNameLength && 2*stem >= min(len(a), len(b))
	return distance, related || distance <= typoTolerance(a)
}

func sharedPrefixLength(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func typoTolerance(name string) int {
	return min(2, len(name)/3)
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			substitution := previous[j-1]
			if a[i-1] != b[j-1] {
				substitution++
			}
			current[j] = min(previous[j]+1, current[j-1]+1, substitution)
		}
		previous = current
	}
	return previous[len(b)]
}

// validateTypedArgs rejects an argument the caller did pass in a shape the
// accessors cannot read. Dropping it instead is what let a malformed deltaY
// degrade into a scroll with no magnitude — or, with direction set, into a
// confidently wrong scroll the other way, which the caller cannot detect.
func validateTypedArgs(toolName string, args map[string]any) error {
	types := schemaArgTypesOnce()[toolName]
	if len(types) == 0 || len(args) == 0 {
		return nil
	}

	bad := make([]string, 0, len(args))
	for name, declared := range types {
		value, present := args[name]
		if !present || value == nil {
			continue
		}
		if raw, isString := value.(string); isString && strings.TrimSpace(raw) == "" {
			continue
		}
		if typedArgIsReadable(declared, value) {
			continue
		}
		bad = append(bad, fmt.Sprintf("%s expects a %s, got %s", name, declared, describeArgValue(value)))
	}
	if len(bad) == 0 {
		return nil
	}

	sort.Strings(bad)
	return fmt.Errorf("%s: %s", toolName, strings.Join(bad, "; "))
}

func typedArgIsReadable(declared string, value any) bool {
	switch declared {
	case "number", "integer":
		if _, ok := value.(float64); ok {
			return true
		}
		raw, ok := value.(string)
		if !ok {
			return false
		}
		_, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		return err == nil
	case "boolean":
		if _, ok := value.(bool); ok {
			return true
		}
		raw, ok := value.(string)
		if !ok {
			return false
		}
		_, err := strconv.ParseBool(strings.TrimSpace(raw))
		return err == nil
	}
	return true
}

// describeArgValue echoes what arrived so the model can correct itself in one
// turn, quoting strings so a numeric-looking string is distinguishable from a
// number.
func describeArgValue(value any) string {
	if raw, ok := value.(string); ok {
		return strconv.Quote(raw)
	}
	return fmt.Sprintf("%v", value)
}

// withTypedArgChecks rejects malformed arguments before the handler runs, so an
// unreadable value never reaches upstream and every accessor's (_, false) again
// means only "absent" — which is what all of their call sites already assume.
func withTypedArgChecks(name string, h func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := validateDeclaredArgs(name, r.GetArguments()); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := validateTypedArgs(name, r.GetArguments()); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return h(ctx, r)
	}
}
