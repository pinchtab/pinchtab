package e2e

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Args struct {
	Suite     string
	Filter    string
	Test      string
	Extra     string
	Logs      string
	Provider  string
	Providers []string // resolved list; len>1 means matrix mode
	DryRun    bool
}

var errHelp = errors.New("help requested")

const usageText = `Usage:
  runner e2e --suite basic [options]
  runner e2e --suite extended [options]
  runner e2e --suite smoke [options]
  runner e2e --suite infra-extended --filter orchestrator

Options:
  --suite basic|extended|smoke|api|cli|infra|plugin|api-extended|cli-extended|infra-extended
          smoke-orchestrator|smoke-security|smoke-lifecycle
  --filter TEXT          Filter scenario file names, groups, tiers, helpers, or tags
  --test TEXT            Run one start_test block by name
  --extra FILES          Add extra scenario files, space-separated
  --logs compact|show|hide
                         Control output verbosity (default: compact).
                         compact: single-line progress per step.
                         show: full streaming output. hide: alias for compact.
  --browser chrome|cloak|ghost-chrome|all
                         Select browser (default: chrome). Use "all" or
                         comma-separated names to run a browser matrix. Cloak
                         builds pinchtab-cloakbrowser:test unless SKIP_BUILD=1.
                         ghost-chrome uses Chrome with static routing.
  --dry-run              Print the compose plan without running it
  --help, -h             Show this help
`

func Run(argv []string, stdout, stderr io.Writer) int {
	args, err := ParseArgs(argv)
	if err != nil {
		if errors.Is(err, errHelp) {
			WriteUsage(stdout)
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "e2e: %v\n\n", err)
		WriteUsage(stderr)
		return 1
	}

	// Matrix mode: run once per provider, tearing down between each.
	if len(args.Providers) > 1 {
		return runProviderMatrix(args, stdout, stderr)
	}

	r, err := NewRunner(args, stdout, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "e2e: %v\n", err)
		return 1
	}
	return r.Run()
}

// runProviderMatrix iterates over each provider in args.Providers, runs
// the full suite for that provider, and reports a combined result.
func runProviderMatrix(args Args, stdout, stderr io.Writer) int {
	_, _ = fmt.Fprintf(stdout, "runner e2e (Go) - browser matrix: %s\n\n",
		strings.Join(args.Providers, ", "))

	worstCode := 0
	for i, provider := range args.Providers {
		if i > 0 {
			_, _ = fmt.Fprintln(stdout, "")
			_, _ = fmt.Fprintln(stdout, strings.Repeat("─", 60))
		}
		_, _ = fmt.Fprintf(stdout, "== browser matrix [%d/%d]: %s ==\n\n",
			i+1, len(args.Providers), provider)

		single := args
		single.Provider = provider
		single.Providers = []string{provider}

		r, err := NewRunner(single, stdout, stderr)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "e2e: browser %s: %v\n", provider, err)
			worstCode = 1
			continue
		}
		if code := r.Run(); code != 0 {
			worstCode = code
		}
	}

	_, _ = fmt.Fprintln(stdout, "")
	if worstCode == 0 {
		_, _ = fmt.Fprintf(stdout, "Browser matrix completed: all %d browsers passed\n",
			len(args.Providers))
	} else {
		_, _ = fmt.Fprintf(stdout, "Browser matrix completed: one or more browsers failed\n")
	}
	return worstCode
}

func ParseArgs(argv []string) (Args, error) {
	args := Args{Suite: "basic"}

	next := func(i *int, name string) (string, error) {
		*i = *i + 1
		if *i >= len(argv) {
			return "", fmt.Errorf("%s requires a value", name)
		}
		return argv[*i], nil
	}

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		// Normalize --flag=value into --flag value so each case below
		// keeps a single shape.
		if idx := strings.Index(arg, "="); idx > 2 && strings.HasPrefix(arg, "--") {
			value := arg[idx+1:]
			arg = arg[:idx]
			argv = append(argv[:i+1], append([]string{value}, argv[i+1:]...)...)
			argv[i] = arg
		}
		switch arg {
		case "--help", "-h":
			return args, errHelp
		case "--suite":
			v, err := next(&i, arg)
			if err != nil {
				return args, err
			}
			args.Suite = v
		case "--filter":
			v, err := next(&i, arg)
			if err != nil {
				return args, err
			}
			args.Filter = v
		case "--test":
			v, err := next(&i, arg)
			if err != nil {
				return args, err
			}
			args.Test = v
		case "--extra":
			v, err := next(&i, arg)
			if err != nil {
				return args, err
			}
			args.Extra = v
		case "--logs":
			v, err := next(&i, arg)
			if err != nil {
				return args, err
			}
			args.Logs = v
		case "--browser":
			v, err := next(&i, arg)
			if err != nil {
				return args, err
			}
			args.Provider = v
		case "--dry-run":
			args.DryRun = true
		default:
			return args, fmt.Errorf("unknown option: %s", arg)
		}
	}

	args.Suite = strings.TrimSpace(args.Suite)
	if args.Suite == "" {
		return args, errors.New("--suite cannot be empty")
	}
	if args.Logs != "" {
		switch args.Logs {
		case "show", "hide", "compact":
		default:
			return args, fmt.Errorf("--logs must be compact, show, or hide")
		}
	}
	if args.Provider == "" {
		args.Provider = "chrome"
	}
	args.Providers = resolveProviderList(args.Provider)
	if len(args.Providers) == 0 {
		return args, fmt.Errorf("--browser must be chrome, cloak, ghost-chrome, all, or a comma-separated list (got %q)", args.Provider)
	}
	for _, p := range args.Providers {
		switch p {
		case "chrome", "cloak", "ghost-chrome":
		default:
			return args, fmt.Errorf("--browser list contains unknown browser %q (got %q)", p, args.Provider)
		}
	}
	return args, nil
}

// resolveProviderList expands the --browser value into a deduplicated
// ordered list.  "all" expands to ["chrome","cloak","ghost-chrome"].
func resolveProviderList(raw string) []string {
	if raw == "all" {
		return []string{"chrome", "cloak", "ghost-chrome"}
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func normalizeSuite(raw string) (string, error) {
	switch raw {
	case "basic", "extended":
		return raw, nil
	case "api", "cli", "infra", "plugin",
		"api-extended", "cli-extended", "infra-extended",
		"smoke", "smoke-orchestrator", "smoke-security", "smoke-lifecycle":
		return raw, nil
	default:
		return "", fmt.Errorf("unknown suite %q", raw)
	}
}

func WriteUsage(w io.Writer) {
	_, _ = io.WriteString(w, usageText)
}

func resolveRepoRoot() string {
	cwd, _ := os.Getwd()
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

func shellQuoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return r != '/' && r != '-' && r != '_' && r != '.' && r != '=' && r != ':' &&
			(r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z')
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
