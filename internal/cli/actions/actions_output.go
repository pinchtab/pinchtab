package actions

import (
	"encoding/json"
	"fmt"
	"os"
)

func exitErr(code int, msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(code)
}

func requireBytes(result []byte, code int, failMsg string) []byte {
	if result == nil {
		exitErr(code, failMsg)
	}
	return result
}

func requireMap(result map[string]any, code int, failMsg string) map[string]any {
	if result == nil {
		exitErr(code, failMsg)
	}
	return result
}

func decodeMap(result []byte, code int, errPrefix string) map[string]any {
	var buf map[string]any
	if err := json.Unmarshal(result, &buf); err != nil {
		exitErr(code, fmt.Sprintf("%s: %v", errPrefix, err))
	}
	return buf
}

func printIndented(v any) {
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
}
