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
	result := apiclient.DoPostQuiet(conn.Client, conn.Base, conn.Token, path, body, capture)
	output.JSON(result["data"])
	if withFields || explain {
		fields, _ := result["fields"].(map[string]any)
		for _, row := range extractFieldRows(fields, "", explain) {
			fmt.Println(row)
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

func extractFieldRows(fields map[string]any, prefix string, explain bool) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	var rows []string
	for _, name := range names {
		field, _ := fields[name].(map[string]any)
		rows = append(rows, extractFieldRow(prefix+name, field, explain))
		items, _ := field["items"].([]any)
		for i, it := range items {
			item, _ := it.(map[string]any)
			label := prefix + name + "[" + strconv.Itoa(i) + "]"
			ref, _ := item["ref"].(string)
			rows = append(rows, label+"\t"+orDash(ref))
			itemFields, _ := item["fields"].(map[string]any)
			rows = append(rows, extractFieldRows(itemFields, label+".", explain)...)
		}
	}
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
