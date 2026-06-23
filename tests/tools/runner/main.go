package main

import (
	"fmt"
	"io"
	"os"

	"github.com/pinchtab/pinchtab/tests/tools/runner/internal/bench"
	"github.com/pinchtab/pinchtab/tests/tools/runner/internal/e2e"
	"github.com/pinchtab/pinchtab/tests/tools/runner/internal/opt"
	"github.com/pinchtab/pinchtab/tests/tools/runner/internal/stealth"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		return bench.Run(argv, stdout, stderr)
	}

	switch argv[0] {
	case "e2e":
		return e2e.Run(argv[1:], stdout, stderr)
	case "bench":
		return bench.Run(argv[1:], stdout, stderr)
	case "browserbench":
		return bench.RunBrowserBench(argv[1:], stdout, stderr)
	case "verify-step", "record-step", "step-end":
		return bench.Run(argv, stdout, stderr)
	case "opt":
		return opt.Run(argv[1:], stdout, stderr)
	case "merge-reports", "verify-answers", "inject-usage":
		return opt.Run(argv, stdout, stderr)
	case "stealth":
		return stealth.Run(argv[1:], stdout, stderr)
	default:
		if argv[0] == "--lane" || argv[0] == "--help" || argv[0] == "-h" {
			return bench.Run(argv, stdout, stderr)
		}
		if len(argv[0]) > 2 && argv[0][:2] == "--" {
			return bench.Run(argv, stdout, stderr)
		}
		_, _ = fmt.Fprintf(stderr, "runner: unknown subcommand %q\n\n", argv[0])
		_, _ = fmt.Fprintln(stderr, "Use one of:")
		_, _ = fmt.Fprintln(stderr, "  runner e2e --suite basic")
		_, _ = fmt.Fprintln(stderr, "  runner bench --lane pinchtab")
		_, _ = fmt.Fprintln(stderr, "  runner browserbench --tasks 5")
		_, _ = fmt.Fprintln(stderr, "  runner --lane pinchtab")
		_, _ = fmt.Fprintln(stderr, "  runner step-end ...")
		_, _ = fmt.Fprintln(stderr, "  runner opt merge-reports ...")
		_, _ = fmt.Fprintln(stderr, "  runner opt verify-answers ...")
		_, _ = fmt.Fprintln(stderr, "  runner stealth compare report1.json report2.json")
		return 1
	}
}
