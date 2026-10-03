package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

const schemaFromStdin = "-"

func Extract(conn apiclient.Conn, cmd *cobra.Command) {
	tabID, _ := cmd.Flags().GetString("tab")
	schemaPath, _ := cmd.Flags().GetString("schema")
	scope, _ := cmd.Flags().GetString("scope")
	maxItems, _ := cmd.Flags().GetInt("max-items")
	withFields, _ := cmd.Flags().GetBool("fields")
	explain, _ := cmd.Flags().GetBool("explain")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	schema, err := readExtractSchema(schemaPath, cmd.InOrStdin())
	if err != nil {
		cli.Fatal("%v", err)
	}
	body := map[string]any{"schema": schema}
	if scope = strings.TrimSpace(scope); scope != "" {
		body["scope"] = scope
	}
	if maxItems > 0 {
		body["maxItems"] = maxItems
	}

	path := apiclient.TabPath(tabID, "extract")
	capture := apiclient.CaptureVocab(tabID == "")

	if jsonOutput {
		conn.Post(path, body, capture)
		return
	}
	result := conn.PostQuiet(path, body, capture)
	output.JSON(result["data"])
	if withFields || explain {
		fields, _ := result["fields"].(map[string]any)
		for _, row := range extractFieldTable(fields, explain) {
			fmt.Fprintln(os.Stderr, row)
		}
	}
	if !explain {
		data, _ := result["data"].(map[string]any)
		fields, _ := result["fields"].(map[string]any)
		for _, line := range extractUnmatchedLines(data, fields) {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	if missing := stringList(result["missing"]); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing required fields: %s\n", strings.Join(missing, ", "))
	}
	if truncated, _ := result["truncated"].(bool); truncated {
		fmt.Fprintln(os.Stderr, "truncated: an array hit its item cap; raise --max-items or the schema's maxItems for more")
	}
}

func readExtractSchema(path string, stdin io.Reader) (json.RawMessage, error) {
	source := path
	var raw []byte
	var err error
	if path == schemaFromStdin {
		source = "stdin"
		raw, err = io.ReadAll(stdin)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read --schema from %s: %w", source, err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("--schema from %s is empty; pass a JSON schema object", source)
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("--schema from %s is not valid JSON", source)
	}
	return json.RawMessage(raw), nil
}

type unmatchedField struct{ path, reason string }

func extractUnmatchedLines(data, fields map[string]any) []string {
	visits := map[string]int{}
	failures := map[unmatchedField]int{}
	walkExtractFields(fields, data, "", "", func(v extractFieldVisit) {
		if v.item {
			return
		}
		visits[v.path]++
		if !v.matched {
			reason, _ := v.field["reason"].(string)
			failures[unmatchedField{v.path, orDash(reason)}]++
		}
	})

	keys := make([]unmatchedField, 0, len(failures))
	for key := range failures {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].reason < keys[j].reason
	})

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		detail := key.reason
		if strings.Contains(key.path, "[]") {
			detail += " in " + strconv.Itoa(failures[key]) + "/" + strconv.Itoa(visits[key.path]) + " items"
		}
		lines = append(lines, "unmatched: "+key.path+" ("+detail+"); rerun with --explain")
	}
	return lines
}

func extractFieldColumns(explain bool) []string {
	columns := []string{"field", "ref", "confidence"}
	if explain {
		columns = append(columns, "score", "source", "reason")
	}
	return columns
}

func extractFieldTable(fields map[string]any, explain bool) []string {
	header := strings.Join(extractFieldColumns(explain), "\t")
	return append([]string{header}, extractFieldRows(fields, explain)...)
}

type extractFieldVisit struct {
	label, path string
	field       map[string]any
	matched     bool
	item        bool
}

func walkExtractFields(fields, data map[string]any, label, path string, visit func(extractFieldVisit)) {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		field, _ := fields[name].(map[string]any)
		value, matched := data[name]
		visit(extractFieldVisit{label: label + name, path: path + name, field: field, matched: matched})
		values, _ := value.([]any)
		items, _ := field["items"].([]any)
		for i, it := range items {
			item, _ := it.(map[string]any)
			itemLabel := label + name + "[" + strconv.Itoa(i) + "]"
			visit(extractFieldVisit{label: itemLabel, path: path + name + "[]", field: item, item: true})
			var itemData map[string]any
			if i < len(values) {
				itemData, _ = values[i].(map[string]any)
			}
			itemFields, _ := item["fields"].(map[string]any)
			walkExtractFields(itemFields, itemData, itemLabel+".", path+name+"[].", visit)
		}
	}
}

func extractFieldRows(fields map[string]any, explain bool) []string {
	var rows []string
	walkExtractFields(fields, nil, "", "", func(v extractFieldVisit) {
		if v.item {
			ref, _ := v.field["ref"].(string)
			rows = append(rows, extractItemRow(v.label, ref, explain))
			return
		}
		rows = append(rows, extractFieldRow(v.label, v.field, explain))
	})
	return rows
}

func extractFieldRow(label string, field map[string]any, explain bool) string {
	ref, _ := field["ref"].(string)
	confidence, _ := field["confidence"].(string)
	cols := []string{label, orDash(ref), orDash(confidence)}
	if explain {
		score, _ := field["score"].(float64)
		source, _ := field["source"].(string)
		reason, _ := field["reason"].(string)
		cols = append(cols, strconv.FormatFloat(score, 'f', 2, 64), orDash(source), orDash(reason))
	}
	return strings.Join(cols, "\t")
}

func extractItemRow(label, ref string, explain bool) string {
	cols := []string{label, orDash(ref)}
	for range len(extractFieldColumns(explain)) - len(cols) {
		cols = append(cols, "-")
	}
	return strings.Join(cols, "\t")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
