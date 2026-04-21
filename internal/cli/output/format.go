package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// Success prints "OK" for commands that succeeded with no payload.
func Success() {
	fmt.Println("OK")
}

// Value prints a single scalar value.
func Value(v any) {
	fmt.Println(v)
}

// Error prints an error to stderr and exits with the given code.
// Format: ERROR: <cmd>: <reason>
func Error(cmd, reason string, code int) {
	fmt.Fprintf(os.Stderr, "ERROR: %s: %s\n", cmd, reason)
	os.Exit(code)
}

// ErrorWithHint prints an error with a hint to stderr and exits.
func ErrorWithHint(cmd, reason, hint string, code int) {
	fmt.Fprintf(os.Stderr, "ERROR: %s: %s\n", cmd, reason)
	fmt.Fprintf(os.Stderr, "HINT: %s\n", hint)
	os.Exit(code)
}

// Hint prints a hint line to stderr (does not exit).
func Hint(text string) {
	fmt.Fprintf(os.Stderr, "HINT: %s\n", text)
}

// JSON prints the value as pretty-printed JSON.
func JSON(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(data))
}

// Exit codes per the spec.
const (
	ExitSuccess     = 0
	ExitUsage       = 1 // bad flags, missing arg
	ExitRuntime     = 2 // server returned non-2xx
	ExitNotFound    = 3 // ref/selector didn't resolve
	ExitTimeout     = 4 // wait/nav exceeded budget
	ExitIDPIBlocked = 5 // blocked by IDPI
)
