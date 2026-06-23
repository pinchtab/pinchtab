// Package stealth implements the `runner stealth` subcommand family.
//
// It aggregates per-provider stealth-score JSON reports produced by the
// /pinchtab-stealth-score skill, prints a side-by-side markdown comparison,
// and appends a one-line summary to history.jsonl so cross-session trends
// can be inspected.
package stealth

import (
	"fmt"
	"io"
)

func Run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: runner stealth <compare> [args...]")
		return 1
	}

	switch argv[0] {
	case "compare":
		return RunCompare(argv[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "runner stealth: unknown subcommand %q\n", argv[0])
		_, _ = fmt.Fprintln(stderr, "Use: compare")
		return 1
	}
}
