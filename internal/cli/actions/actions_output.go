package actions

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
)

func exitErr(code int, msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(code)
}

func usageErr(format string, args ...any) {
	exitErr(cli.ExitUsage, fmt.Sprintf(format, args...))
}

func runtimeErr(format string, args ...any) {
	exitErr(cli.ExitRuntime, fmt.Sprintf(format, args...))
}

func requireBytes(result []byte, failMsg string) []byte {
	if result == nil {
		runtimeErr("%s", failMsg)
	}
	return result
}

func requireMap(result map[string]any, failMsg string) map[string]any {
	if result == nil {
		runtimeErr("%s", failMsg)
	}
	return result
}

func decodeMap(result []byte, errPrefix string) map[string]any {
	var buf map[string]any
	if err := json.Unmarshal(result, &buf); err != nil {
		runtimeErr("%s: %v", errPrefix, err)
	}
	return buf
}

func printIndented(v any) {
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
}
