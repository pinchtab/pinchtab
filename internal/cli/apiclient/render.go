package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func PrintAndDecode(body []byte) map[string]any {
	return printAndDecode(body)
}

// printAndDecode pretty-prints the body when it is JSON, falls back to
// raw output otherwise, and returns the parsed map (if any) for the
// suggestion logic. It only warns on genuine JSON decode errors — inherently
// non-JSON responses like /snapshot's compact text format pass silently.
func printAndDecode(body []byte) map[string]any {
	var buf bytes.Buffer
	isJSON := json.Indent(&buf, body, "", "  ") == nil
	if isJSON {
		fmt.Println(buf.String())
	} else {
		fmt.Println(string(body))
	}
	if !isJSON {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err == nil {
		return result
	}
	// Body is valid JSON but not an object (array, string, number, etc.).
	// That's fine — many endpoints return arrays. Don't warn.
	return nil
}
